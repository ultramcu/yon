package ui

import (
	"net/url"
	"strings"

	"github.com/ultramcu/yon/internal/model"
)

// This file holds the pure helpers behind the two-way URL⇄Params sync in the
// request editor: the URL field's query string and the Params table mirror each
// other. They are kept Fyne-free so they can be unit-tested directly.

// splitURLQuery splits raw at the first '?' into the part before it (base) and
// the query string after it. With no '?', query is "".
func splitURLQuery(raw string) (base, query string) {
	if i := strings.IndexByte(raw, '?'); i >= 0 {
		return raw[:i], raw[i+1:]
	}
	return raw, ""
}

// joinURLQuery is the inverse of splitURLQuery: base, optionally followed by
// "?"+query when query is non-empty.
func joinURLQuery(base, query string) string {
	if query == "" {
		return base
	}
	return base + "?" + query
}

// parseQueryParams parses a query string ("a=1&b=2") into ordered, enabled
// Params, percent-decoding keys and values so pasted/encoded URLs show readable
// text. A bare key ("a") yields an empty value; empty segments are skipped.
func parseQueryParams(query string) []model.Param {
	var out []model.Param
	for _, seg := range strings.Split(query, "&") {
		if seg == "" {
			continue
		}
		key, val, _ := strings.Cut(seg, "=")
		out = append(out, model.Param{
			Key:     queryUnescape(key),
			Value:   queryUnescape(val),
			Enabled: true,
		})
	}
	return out
}

// encodeQueryParams renders the enabled params back into a query string for the
// URL box. Keys/values are percent-encoded so a value containing query
// metacharacters (& = # space %) round-trips losslessly back through
// parseQueryParams — but the {{template}} delimiters are then restored to literal
// braces so templates stay readable in the URL box. Fully empty and disabled
// rows are skipped. The real wire encoding is still done by yonner on send.
func encodeQueryParams(params []model.Param) string {
	var b strings.Builder
	for _, p := range params {
		if !p.Enabled || (p.Key == "" && p.Value == "") {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('&')
		}
		b.WriteString(queryEscapeReadableTemplates(p.Key))
		b.WriteByte('=')
		b.WriteString(queryEscapeReadableTemplates(p.Value))
	}
	return b.String()
}

// uiTemplateBraceRestorer turns the percent-encoded {{ }} delimiters back into
// literal braces (Go emits uppercase hex; lowercase handled for safety). Mirrors
// yonner.restoreTemplateBraces but kept local to avoid a cross-package export.
var uiTemplateBraceRestorer = strings.NewReplacer(
	"%7B%7B", "{{", "%7b%7b", "{{",
	"%7D%7D", "}}", "%7d%7d", "}}",
)

// queryEscapeReadableTemplates percent-encodes s for the URL query, then leaves
// {{template}} delimiters readable. parseQueryParams' QueryUnescape reverses the
// encoding exactly, so the Params⇄URL mirror is lossless.
func queryEscapeReadableTemplates(s string) string {
	return uiTemplateBraceRestorer.Replace(url.QueryEscape(s))
}

// mergeQueryIntoParams rebuilds the Params list when the URL query changes: the
// parsed query params fill the table's enabled slots in order, while disabled
// rows are preserved in their original positions ("kept but not sent"). Any
// query params beyond the existing enabled slots (e.g. a freshly appended
// &key=val) are added at the end.
func mergeQueryIntoParams(old, query []model.Param) []model.Param {
	out := make([]model.Param, 0, len(old)+len(query))
	qi := 0
	for _, p := range old {
		if p.Enabled {
			if qi < len(query) { // replace this enabled slot with the next query param
				out = append(out, query[qi])
				qi++
			}
			// else: an enabled row with no matching query param was removed from the URL — drop it
		} else {
			out = append(out, p) // disabled row stays where it is
		}
	}
	for ; qi < len(query); qi++ { // brand-new params appended in the URL
		out = append(out, query[qi])
	}
	return out
}

// queryUnescape decodes a percent-encoded query token, leaving it unchanged if
// it is not valid encoding (so a literal value with a stray '%' is preserved).
func queryUnescape(s string) string {
	if d, err := url.QueryUnescape(s); err == nil {
		return d
	}
	return s
}
