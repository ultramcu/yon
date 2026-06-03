package ui

import (
	"bytes"
	"strings"
	"testing"
)

// TestFormatXML_PreservesReservedXMLPrefix pins defect D1: attributes in the
// implicit XML namespace (xml:lang, xml:space, …) must keep their "xml:" prefix.
func TestFormatXML_PreservesReservedXMLPrefix(t *testing.T) {
	for _, in := range []string{
		`<html xml:lang="en"><body/></html>`,
		`<doc xml:space="preserve"><p/></doc>`,
	} {
		out, ok := formatXML([]byte(in))
		if !ok {
			t.Fatalf("formatXML(%q) ok=false", in)
		}
		want := strings.SplitN(strings.SplitN(in, ":", 2)[1], "=", 2)[0] // lang | space
		if !strings.Contains(string(out), "xml:"+want+"=") {
			t.Errorf("formatXML(%q) dropped the reserved xml: prefix:\n%s", in, out)
		}
	}
}

// TestFormatXML_Idempotent pins defect D2: formatting already-formatted output
// must be stable (no accumulating whitespace), including on mixed content.
func TestFormatXML_Idempotent(t *testing.T) {
	for _, in := range []string{
		`<a><b>1</b><b>2</b></a>`,      // element tree
		`<p>hi <b>there</b> world</p>`, // mixed content
		`<a><!-- c -->text</a>`,        // comment + text
		`<p>x<a/> mid <b/>y</p>`,       // text between/around children
		`<root><item>1</item><item>2</item></root>`,
	} {
		once, ok := formatXML([]byte(in))
		if !ok {
			t.Fatalf("formatXML(%q) ok=false", in)
		}
		twice, ok := formatXML(once)
		if !ok {
			t.Fatalf("formatXML(format(%q)) ok=false", in)
		}
		if !bytes.Equal(once, twice) {
			t.Errorf("formatXML not idempotent for %q:\n--- once ---\n%s\n--- twice ---\n%s", in, once, twice)
		}
	}
}
