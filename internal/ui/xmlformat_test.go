package ui

// Blind tests (Test B) for the XML formatter + XML response rendering.
// Written from the v0.7.0 contract spec only, not from the implementation:
//
//   formatXML(src []byte) (out []byte, ok bool)
//       Pretty-indents WELL-FORMED XML with a 2-space indent; returns ok=false
//       for malformed XML without mangling. Comments are preserved.
//   isXMLContentType(ct string) bool
//       true for application/xml, text/xml, application/*+xml (case-insensitive,
//       ignoring a ;charset=... suffix).
//   isHTMLContentType(ct string) bool
//       true for text/html, application/xhtml+xml.
//   responseView with an XML content-type + Pretty on renders the body
//       pretty-printed (indented) in the TextGrid.

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/ultramcu/yon/internal/model"
)

// stripWS removes all ASCII whitespace so two XML serializations can be
// compared for "same elements, different layout".
func stripWS(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\n', '\r':
			return -1
		}
		return r
	}, s)
}

// TestFormatXML_IndentsNestedElements pins that well-formed XML is actually
// indented: ok==true, the output gains newlines + 2-space indentation, and the
// element content is unchanged once whitespace is stripped. A no-op formatter
// (returns input as-is) would fail the "\n  <b>" assertion.
func TestFormatXML_IndentsNestedElements(t *testing.T) {
	in := []byte(`<a><b>1</b><b>2</b></a>`)
	out, ok := formatXML(in)
	if !ok {
		t.Fatalf("formatXML(well-formed) ok = false, want true")
	}
	got := string(out)
	if !strings.Contains(got, "\n") {
		t.Fatalf("output not multi-line:\n%q", got)
	}
	if !strings.Contains(got, "\n  <b>") {
		t.Fatalf("output not 2-space indented; missing %q in:\n%q", "\n  <b>", got)
	}
	// Same elements, just reformatted.
	if w := stripWS(got); w != "<a><b>1</b><b>2</b></a>" {
		t.Fatalf("reformatted content changed: stripWS = %q, want %q", w, "<a><b>1</b><b>2</b></a>")
	}
}

// TestFormatXML_PreservesComment pins the "don't drop comments" requirement: an
// XML comment in the input must survive into the pretty-printed output.
func TestFormatXML_PreservesComment(t *testing.T) {
	in := []byte(`<a><!-- keep me --><b/></a>`)
	out, ok := formatXML(in)
	if !ok {
		t.Fatalf("formatXML(comment input) ok = false, want true")
	}
	if !strings.Contains(string(out), "<!-- keep me -->") {
		t.Fatalf("comment dropped/mangled; want %q in:\n%q", "<!-- keep me -->", string(out))
	}
}

// TestFormatXML_MalformedReturnsFalse pins that mismatched tags are reported as
// not-ok (ok==false) and do not panic. (The body need not be preserved here;
// only the ok=false signal is contracted for malformed input.)
func TestFormatXML_MalformedReturnsFalse(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("formatXML(malformed) panicked: %v", r)
		}
	}()
	_, ok := formatXML([]byte(`<a><b></a>`))
	if ok {
		t.Fatalf("formatXML(mismatched tags) ok = true, want false")
	}
}

// TestFormatXML_EmptyAndWhitespace pins safe handling of empty / whitespace-only
// input: no panic. Per the contract there is no well-formed document here, so we
// assume ok==false (stated assumption).
func TestFormatXML_EmptyAndWhitespace(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("formatXML(empty/ws) panicked: %v", r)
		}
	}()
	for _, in := range []string{"", "   ", "\n\t  \n"} {
		if _, ok := formatXML([]byte(in)); ok {
			t.Errorf("formatXML(%q) ok = true, want false (no document)", in)
		}
	}
}

// TestIsXMLContentType pins XML content-type recognition: the +xml family and
// text/xml, case-insensitive and ignoring a charset suffix; non-XML types and
// the empty string are false.
func TestIsXMLContentType(t *testing.T) {
	for _, ct := range []string{
		"application/xml",
		"text/xml",
		"application/soap+xml",
		"application/xml; charset=utf-8",
		"APPLICATION/XML",
	} {
		if !isXMLContentType(ct) {
			t.Errorf("isXMLContentType(%q) = false, want true", ct)
		}
	}
	for _, ct := range []string{
		"application/json",
		"text/html",
		"",
	} {
		if isXMLContentType(ct) {
			t.Errorf("isXMLContentType(%q) = true, want false", ct)
		}
	}
}

// TestIsHTMLContentType pins HTML recognition: text/html (with/without charset)
// is true; an XML type is false.
func TestIsHTMLContentType(t *testing.T) {
	for _, ct := range []string{
		"text/html",
		"text/html; charset=utf-8",
	} {
		if !isHTMLContentType(ct) {
			t.Errorf("isHTMLContentType(%q) = false, want true", ct)
		}
	}
	if isHTMLContentType("application/xml") {
		t.Errorf("isHTMLContentType(application/xml) = true, want false")
	}
}

// TestResponseView_XMLPrettyRendersIndented pins the end-to-end behaviour: an
// XML response with Pretty on is shown pretty-printed (indented) in the body
// TextGrid — the rendered text is multi-line and contains indentation, not the
// original single line "<a><b>1</b></a>".
func TestResponseView_XMLPrettyRendersIndented(t *testing.T) {
	test.NewApp()
	w := test.NewWindow(nil)
	t.Cleanup(w.Close)
	rv := newResponseView(w)

	rv.setPretty(true)
	rv.setResponse(model.Response{
		Status: 200,
		Headers: []model.Param{
			{Key: "Content-Type", Value: "application/xml", Enabled: true},
		},
		Body: []byte("<a><b>1</b></a>"),
	})

	got := rv.bodyGrid.Text()
	if !strings.Contains(got, "\n") {
		t.Fatalf("XML pretty render not multi-line; got %q", got)
	}
	if !strings.Contains(got, "  <b>") {
		t.Fatalf("XML pretty render not indented; missing %q in %q", "  <b>", got)
	}
	if got == "<a><b>1</b></a>" {
		t.Fatalf("body rendered verbatim (not pretty-printed): %q", got)
	}
	// Sanity: pretty-printing must not change the element content.
	if w := stripWS(got); w != "<a><b>1</b></a>" {
		t.Fatalf("pretty render changed content: stripWS = %q, want %q", w, "<a><b>1</b></a>")
	}
}
