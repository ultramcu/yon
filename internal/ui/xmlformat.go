package ui

import (
	"bytes"
	"encoding/xml"
	"errors"
	"image/color"
	"io"
	"strings"

	"fyne.io/fyne/v2/widget"
)

// XML/HTML syntax-highlight colours. Tuned, like the JSON palette, to read on
// both light and dark themes (mid-saturation hues that contrast against either
// background). Tag delimiters and element names share the JSON "key" blue so
// markup structure pops the way object keys do; attribute names reuse the JSON
// number orange, attribute values the JSON string green, and comments the
// muted JSON punctuation grey.
var (
	colorXMLTag       = color.NRGBA{R: 0x4f, G: 0x9c, B: 0xff, A: 0xff} // blue   (< > / and element names)
	colorXMLAttrName  = color.NRGBA{R: 0xe0, G: 0x80, B: 0x2b, A: 0xff} // orange (attribute names)
	colorXMLAttrValue = color.NRGBA{R: 0x4c, G: 0xaf, B: 0x50, A: 0xff} // green  (attribute values)
	colorXMLComment   = color.NRGBA{R: 0x9a, G: 0x9a, B: 0x9a, A: 0xff} // grey   (<!-- ... --> and PIs)
)

// Shared, immutable style objects — one per colour category — reused across
// every coloured cell instead of allocating a fresh *CustomTextGridStyle per
// token, exactly like the JSON styles in jsonpretty.go. TextGrid reads only
// FGColor from a style, so pointing many cells at the same singleton is safe.
var (
	styleXMLTag       = &widget.CustomTextGridStyle{FGColor: colorXMLTag}
	styleXMLAttrName  = &widget.CustomTextGridStyle{FGColor: colorXMLAttrName}
	styleXMLAttrValue = &widget.CustomTextGridStyle{FGColor: colorXMLAttrValue}
	styleXMLComment   = &widget.CustomTextGridStyle{FGColor: colorXMLComment}
)

// formatXML pretty-indents well-formed XML with two-space indentation. It
// returns the reformatted bytes and true on success, or (nil, false) when src
// is not well-formed (a decoder error), so the caller can fall back to showing
// the raw body rather than mangling invalid input.
//
// It walks the token stream with encoding/xml's decoder rather than using
// xml.Encoder, because the standard Encoder drops comments and does not faithfully
// round-trip processing instructions/directives. Re-emitting CharData, Comment,
// ProcInst and Directive tokens by hand preserves them. Insignificant whitespace
// between elements (the indentation of the input) is trimmed so the re-indentation
// is clean; text that is purely whitespace is treated as element formatting and
// dropped, while element content that contains non-whitespace is kept verbatim
// (and the enclosing element is emitted inline so the text is not reflowed).
func formatXML(src []byte) (out []byte, ok bool) {
	root, ok := parseXML(src)
	if !ok || len(root.children) == 0 {
		return nil, false
	}
	// Reject rootless fragments that carry significant top-level text (e.g. a
	// plain-text body that merely contains a stray tag): that is not an XML
	// document, and formatting it is neither meaningful nor idempotent. The
	// caller then shows the raw body instead.
	for _, c := range root.children {
		if c.kind == xnText {
			return nil, false
		}
	}
	var buf bytes.Buffer
	for i, c := range root.children {
		if i > 0 {
			buf.WriteByte('\n')
		}
		renderNode(&buf, c, 0)
	}
	buf.WriteByte('\n')
	return buf.Bytes(), true
}

// xnode kinds.
const (
	xnRoot = iota
	xnElem
	xnText
	xnComment
	xnProcInst
	xnDirective
)

// xnode is a parsed XML node. For elements, open is the fully-rendered start tag
// (e.g. `<soap:Envelope xmlns:soap="...">`) and name is the qname for the close
// tag — both computed at parse time while namespace bindings are in scope, so
// prefixes are preserved exactly. For text/comment/PI/directive, raw holds the
// literal inner content. hasText is true when an element has any significant
// (non-whitespace) text child, marking it mixed content that must render inline.
type xnode struct {
	kind     int
	open     string // element start tag
	name     string // element qname for the close tag
	target   string // ProcInst target
	raw      []byte // text / comment / PI-inst / directive content
	children []*xnode
	hasText  bool
}

// parseXML decodes src into a node tree, returning ok=false on any
// well-formedness error so the caller can fall back to the raw bytes. Pure
// whitespace between elements is dropped (it is re-created as indentation);
// significant text is kept verbatim.
func parseXML(src []byte) (*xnode, bool) {
	dec := xml.NewDecoder(bytes.NewReader(src))
	dec.Strict = true

	root := &xnode{kind: xnRoot}
	stack := []*xnode{root}
	ns := map[string]string{} // resolved URI -> declared prefix

	for {
		tok, err := dec.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, false
		}
		cur := stack[len(stack)-1]
		switch t := tok.(type) {
		case xml.StartElement:
			for _, a := range t.Attr {
				switch {
				case a.Name.Space == "xmlns":
					ns[a.Value] = a.Name.Local
				case a.Name.Space == "" && a.Name.Local == "xmlns":
					ns[a.Value] = ""
				}
			}
			var b bytes.Buffer
			name := writeStartTag(&b, t, ns)
			n := &xnode{kind: xnElem, open: b.String(), name: name}
			cur.children = append(cur.children, n)
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			if isAllSpace(t) {
				continue
			}
			cur.children = append(cur.children, &xnode{kind: xnText, raw: cloneBytes(t)})
			cur.hasText = true
		case xml.Comment:
			cur.children = append(cur.children, &xnode{kind: xnComment, raw: cloneBytes(t)})
		case xml.ProcInst:
			cur.children = append(cur.children, &xnode{kind: xnProcInst, target: t.Target, raw: cloneBytes(t.Inst)})
		case xml.Directive:
			cur.children = append(cur.children, &xnode{kind: xnDirective, raw: cloneBytes(t)})
		}
	}
	return root, true
}

// inlineRender reports whether an element should be emitted on one line with all
// its content verbatim: when it has significant text (mixed/text content) or no
// children at all. Mixed content is rendered inline precisely so no indentation
// whitespace is injected into a text run — which is what makes formatXML
// idempotent and preserves the original inter-node whitespace exactly.
func (n *xnode) inlineRender() bool {
	return n.hasText || len(n.children) == 0
}

// renderNode writes n (block context) at the given indent depth.
func renderNode(buf *bytes.Buffer, n *xnode, depth int) {
	switch n.kind {
	case xnText:
		buf.Write(escapeCharData(n.raw))
	case xnComment:
		buf.WriteString("<!--")
		buf.Write(n.raw)
		buf.WriteString("-->")
	case xnProcInst:
		buf.WriteString("<?")
		buf.WriteString(n.target)
		if len(n.raw) > 0 {
			buf.WriteByte(' ')
			buf.Write(n.raw)
		}
		buf.WriteString("?>")
	case xnDirective:
		buf.WriteString("<!")
		buf.Write(n.raw)
		buf.WriteString(">")
	case xnElem:
		buf.WriteString(n.open)
		if n.inlineRender() {
			renderInline(buf, n)
			writeEndTagName(buf, n.name)
			return
		}
		for _, c := range n.children {
			buf.WriteByte('\n')
			writeSpaces(buf, depth+1)
			renderNode(buf, c, depth+1)
		}
		buf.WriteByte('\n')
		writeSpaces(buf, depth)
		writeEndTagName(buf, n.name)
	}
}

// renderInline writes an element's children with no whitespace injected, so the
// content (and any nested elements) is emitted exactly as parsed.
func renderInline(buf *bytes.Buffer, n *xnode) {
	for _, c := range n.children {
		switch c.kind {
		case xnElem:
			buf.WriteString(c.open)
			renderInline(buf, c)
			writeEndTagName(buf, c.name)
		default:
			renderNode(buf, c, 0)
		}
	}
}

// writeSpaces writes two spaces per indent level.
func writeSpaces(buf *bytes.Buffer, depth int) {
	for i := 0; i < depth; i++ {
		buf.WriteString("  ")
	}
}

// cloneBytes returns a copy of b (decoder token buffers are reused).
func cloneBytes(b []byte) []byte {
	c := make([]byte, len(b))
	copy(c, b)
	return c
}

// writeStartTag emits a start element with its attributes, e.g.
// `<soap:Envelope attr="v" xmlns:soap="...">`, and returns the qname it printed
// so the matching end tag can re-use it. ns maps namespace URIs back to their
// declared prefixes.
func writeStartTag(buf *bytes.Buffer, se xml.StartElement, ns map[string]string) string {
	name := qname(se.Name, ns)
	buf.WriteByte('<')
	buf.WriteString(name)
	for _, a := range se.Attr {
		buf.WriteByte(' ')
		buf.WriteString(attrName(a.Name, ns))
		buf.WriteString(`="`)
		buf.WriteString(escapeAttrValue(a.Value))
		buf.WriteByte('"')
	}
	buf.WriteByte('>')
	return name
}

// writeEndTagName emits `</name>` for the given printed qname.
func writeEndTagName(buf *bytes.Buffer, name string) {
	buf.WriteString("</")
	buf.WriteString(name)
	buf.WriteByte('>')
}

// qname renders an xml.Name as a printable element/qualified name. The decoder
// resolves a prefix into Name.Space (the namespace URI when a matching xmlns is
// in scope, or the raw prefix when it is not). When Space is a URI we map it back
// to the prefix literally declared for it (ns), so `soap:Envelope` is preserved;
// when Space is already a short prefix (no scheme/slash, e.g. for an attribute
// like xml:lang) it is used directly. The xmlns:* declarations themselves are
// preserved as attributes, which is what re-establishes each binding.
func qname(n xml.Name, ns map[string]string) string {
	if n.Space == "" {
		return n.Local
	}
	if p, found := ns[n.Space]; found {
		if p == "" {
			return n.Local // default namespace; no prefix
		}
		return p + ":" + n.Local
	}
	if n.Space == xmlReservedNS {
		return "xml:" + n.Local // the implicit, always-bound `xml` prefix
	}
	if !looksLikeURI(n.Space) {
		return n.Space + ":" + n.Local
	}
	return n.Local
}

// xmlReservedNS is the namespace URI permanently bound to the reserved `xml`
// prefix (xml:lang, xml:space, xml:id, xml:base). The decoder reports it as a
// resolved URI even though no xmlns:xml is ever declared, so it must be mapped
// back to the `xml` prefix explicitly or the prefix would be dropped.
const xmlReservedNS = "http://www.w3.org/XML/1998/namespace"

// attrName renders an attribute's xml.Name. xmlns and xmlns:* declarations are
// reported by the decoder with Space=="xmlns"; reconstruct them literally. Other
// namespaced attributes are mapped back to their declared prefix like qname.
func attrName(n xml.Name, ns map[string]string) string {
	switch {
	case n.Space == "xmlns":
		return "xmlns:" + n.Local
	case n.Space == "" && n.Local == "xmlns":
		return "xmlns"
	case n.Space == "":
		return n.Local
	}
	if p, found := ns[n.Space]; found && p != "" {
		return p + ":" + n.Local
	}
	if n.Space == xmlReservedNS {
		return "xml:" + n.Local // xml:lang, xml:space, xml:id, xml:base
	}
	if !looksLikeURI(n.Space) {
		return n.Space + ":" + n.Local
	}
	return n.Local
}

// looksLikeURI reports whether s appears to be a namespace URI rather than a
// short prefix (so qname/attrName don't emit a URL where a prefix belongs).
func looksLikeURI(s string) bool {
	return strings.Contains(s, ":") || strings.Contains(s, "/")
}

// isAllSpace reports whether b is entirely XML whitespace (space, tab, CR, LF).
func isAllSpace(b []byte) bool {
	for _, c := range b {
		switch c {
		case ' ', '\t', '\r', '\n':
		default:
			return false
		}
	}
	return true
}

// escapeCharData escapes the five characters that must not appear literally in
// element text content, returning the original slice unchanged when nothing
// needs escaping (the common case allocates nothing).
func escapeCharData(b []byte) []byte {
	if !bytes.ContainsAny(b, "<>&") {
		return b
	}
	var out bytes.Buffer
	for _, c := range b {
		switch c {
		case '<':
			out.WriteString("&lt;")
		case '>':
			out.WriteString("&gt;")
		case '&':
			out.WriteString("&amp;")
		default:
			out.WriteByte(c)
		}
	}
	return out.Bytes()
}

// escapeAttrValue escapes a double-quoted attribute value.
func escapeAttrValue(s string) string {
	if !strings.ContainsAny(s, `<>&"`) {
		return s
	}
	var out strings.Builder
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '<':
			out.WriteString("&lt;")
		case '>':
			out.WriteString("&gt;")
		case '&':
			out.WriteString("&amp;")
		case '"':
			out.WriteString("&quot;")
		default:
			out.WriteByte(s[i])
		}
	}
	return out.String()
}

// isXMLContentType reports whether ct is an XML media type: application/xml,
// text/xml, or any application/*+xml or */*+xml suffix type (e.g.
// application/soap+xml, image/svg+xml). The check is case-insensitive and
// ignores any parameters such as `; charset=utf-8`.
func isXMLContentType(ct string) bool {
	mt := mediaType(ct)
	switch mt {
	case "application/xml", "text/xml":
		return true
	}
	return strings.HasSuffix(mt, "+xml")
}

// isHTMLContentType reports whether ct is an HTML media type: text/html or
// application/xhtml+xml. The check is case-insensitive and ignores parameters.
// application/xhtml+xml is XML too, but the response viewer treats it as HTML
// (tag-highlighted, not forcibly reflowed); isHTMLContentType is checked first.
func isHTMLContentType(ct string) bool {
	mt := mediaType(ct)
	return mt == "text/html" || mt == "application/xhtml+xml"
}

// isJSONContentType reports whether ct is a JSON media type: application/json or
// any application/*+json suffix type (e.g. application/problem+json). The check
// is case-insensitive and ignores parameters such as `; charset=utf-8`.
func isJSONContentType(ct string) bool {
	mt := mediaType(ct)
	if mt == "application/json" || mt == "text/json" {
		return true
	}
	return strings.HasSuffix(mt, "+json")
}

// mediaType lower-cases ct, strips any `; parameters` and surrounding spaces,
// and returns the bare `type/subtype`. It is a tiny, allocation-light substitute
// for mime.ParseMediaType (which we avoid to skip its parameter parsing).
func mediaType(ct string) string {
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = ct[:i]
	}
	return strings.ToLower(strings.TrimSpace(ct))
}

// buildXMLRows tokenizes already-formatted XML/HTML text in a SINGLE
// zero-allocation pass and constructs the []widget.TextGridRow directly, the
// same way buildJSONRows does for JSON: each cell's Rune is set and, for
// coloured tokens, its Style points at one of the shared per-colour singletons.
// The caller assigns the result to grid.Rows and Refreshes — bypassing both
// TextGrid.SetText (which reparses every grapheme) and a second styling walk. No
// *CustomTextGridStyle is allocated per token.
//
// Cell layout matches SetText for the markup we display (space-indented, no tabs):
// one cell per rune. Tag delimiters (`<`, `>`, `/`) and element names are coloured
// as tags; attribute names and their `="..."` values are coloured distinctly; and
// `<!-- ... -->` comments / `<? ... ?>` processing instructions are greyed. The
// scanner walks bytes with ASCII fast paths and tracks the rune/cell column so
// multi-byte UTF-8 inside text/values stays aligned.
func buildXMLRows(text string) []widget.TextGridRow {
	n := 1
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			n++
		}
	}
	rows := make([]widget.TextGridRow, 0, n)

	for {
		nl := indexByte(text, '\n')
		var line string
		last := false
		if nl < 0 {
			line = text
			last = true
		} else {
			line = text[:nl]
		}
		rows = append(rows, widget.TextGridRow{Cells: buildXMLCells(line)})
		if last {
			break
		}
		text = text[nl+1:]
	}
	return rows
}

// buildXMLCells builds one row's cells, classifying and colouring markup in a
// single zero-allocation byte pass. It first lays down one cell per rune (Rune
// set, Style nil), then walks the line assigning shared style pointers to the
// cells that belong to a coloured token. Cell columns are tracked explicitly so
// multi-byte runes (one cell each) stay aligned with the byte index.
func buildXMLCells(line string) []widget.TextGridCell {
	cells := make([]widget.TextGridCell, 0, len(line))
	for i := 0; i < len(line); {
		r, sz := decodeRune(line[i:])
		cells = append(cells, widget.TextGridCell{Rune: r})
		i += sz
	}

	set := func(startCol, endCol int, st widget.TextGridStyle) {
		if startCol < 0 {
			startCol = 0
		}
		if endCol >= len(cells) {
			endCol = len(cells) - 1
		}
		for c := startCol; c <= endCol; c++ {
			cells[c].Style = st
		}
	}

	col := 0 // cell (rune) column
	i := 0   // byte index into line
	for i < len(line) {
		if line[i] != '<' {
			// Text content between tags is left in the default foreground.
			_, sz := decodeRune(line[i:])
			i += sz
			col++
			continue
		}

		// A markup construct begins at '<'. Comments and processing
		// instructions are greyed wholesale; element tags are coloured by part.
		if strings.HasPrefix(line[i:], "<!--") {
			startCol := col
			end := strings.Index(line[i:], "-->")
			var endByte int
			if end < 0 {
				endByte = len(line) - 1
			} else {
				endByte = i + end + 2 // include the final '>'
			}
			span := runeCount(line[i : endByte+1])
			set(startCol, startCol+span-1, styleXMLComment)
			i = endByte + 1
			col = startCol + span
			continue
		}
		if strings.HasPrefix(line[i:], "<?") || strings.HasPrefix(line[i:], "<!") {
			startCol := col
			rel := indexByteFrom(line, '>', i)
			var endByte int
			if rel < 0 {
				endByte = len(line) - 1
			} else {
				endByte = i + rel
			}
			span := runeCount(line[i : endByte+1])
			set(startCol, startCol+span-1, styleXMLComment)
			i = endByte + 1
			col = startCol + span
			continue
		}

		// An element tag: colour `<`, optional `/`, the element name and the
		// closing `>`/`/>` as tag; attribute names and `="value"` distinctly.
		i, col = colourElementTag(line, i, col, cells, set)
	}
	return cells
}

// colourElementTag colours one `<...>` element tag starting at byte index i /
// cell column col, returning the byte index and cell column just past the tag's
// closing `>` (or end of line if unterminated). It colours the delimiters and
// element name as tag, attribute names as attr-name, and quoted attribute values
// as attr-value, matching the highlight scheme.
func colourElementTag(line string, i, col int, cells []widget.TextGridCell, set func(int, int, widget.TextGridStyle)) (int, int) {
	// '<' and an optional '/'.
	set(col, col, styleXMLTag)
	i++
	col++
	if i < len(line) && line[i] == '/' {
		set(col, col, styleXMLTag)
		i++
		col++
	}

	// Element name (tag colour): up to whitespace, '>', '/'.
	nameStartCol := col
	for i < len(line) {
		b := line[i]
		if b == ' ' || b == '\t' || b == '>' || b == '/' {
			break
		}
		_, sz := decodeRune(line[i:])
		i += sz
		col++
	}
	if col > nameStartCol {
		set(nameStartCol, col-1, styleXMLTag)
	}

	// Attributes until the tag closes.
	for i < len(line) {
		b := line[i]
		switch {
		case b == ' ' || b == '\t':
			i++
			col++
		case b == '>':
			set(col, col, styleXMLTag)
			i++
			col++
			return i, col
		case b == '/':
			// Possible self-closing '/>'.
			set(col, col, styleXMLTag)
			i++
			col++
		case b == '"' || b == '\'':
			// A bare quoted run (attribute value) — colour as value.
			q := b
			startCol := col
			i++
			col++
			for i < len(line) && line[i] != q {
				_, sz := decodeRune(line[i:])
				i += sz
				col++
			}
			if i < len(line) { // closing quote
				i++
				col++
			}
			set(startCol, col-1, styleXMLAttrValue)
		default:
			// Attribute name up to '=', whitespace, or tag close.
			startCol := col
			for i < len(line) {
				c := line[i]
				if c == '=' || c == ' ' || c == '\t' || c == '>' || c == '/' {
					break
				}
				_, sz := decodeRune(line[i:])
				i += sz
				col++
			}
			if col > startCol {
				set(startCol, col-1, styleXMLAttrName)
			}
			if i < len(line) && line[i] == '=' {
				// colour '=' as attr name's neutral; leave default. Advance.
				i++
				col++
			}
		}
	}
	return i, col
}

// runeCount returns the number of runes in s (== number of TextGrid cells),
// using the ASCII fast path inline so the common case skips the unicode tables.
func runeCount(s string) int {
	n := 0
	for i := 0; i < len(s); {
		if s[i] < 0x80 {
			i++
		} else {
			_, sz := decodeRune(s[i:])
			i += sz
		}
		n++
	}
	return n
}

// indexByteFrom returns the index (relative to start) of the first b at/after
// byte index start in s, or -1. Used to find a tag's closing '>'.
func indexByteFrom(s string, b byte, start int) int {
	for i := start; i < len(s); i++ {
		if s[i] == b {
			return i - start
		}
	}
	return -1
}
