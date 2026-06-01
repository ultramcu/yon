package ui

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// --- prettyJSON: valid JSON is indented; invalid falls back to raw, no panic ---

func TestPrettyJSON_ValidObjectIsIndented(t *testing.T) {
	in := []byte(`{"a":1,"b":[2,3],"c":{"d":true}}`)

	out, ok := prettyJSON(in)
	if !ok {
		t.Fatalf("prettyJSON reported failure on valid JSON")
	}

	// Per the read-only-response rule / responseview: Pretty = indented JSON. Indented output must
	// (a) still be valid, equivalent JSON and (b) actually contain newlines +
	// two-space indentation that the compact input lacked.
	if !strings.Contains(string(out), "\n") {
		t.Errorf("indented output has no newlines: %q", out)
	}
	if !strings.Contains(string(out), "  ") {
		t.Errorf("indented output has no two-space indentation: %q", out)
	}

	// Equivalence check: re-encode both compactly and compare.
	if compact(t, out) != compact(t, in) {
		t.Errorf("indent changed the JSON value:\n got %s\nwant %s",
			compact(t, out), compact(t, in))
	}
}

func TestPrettyJSON_ValidArrayIsIndented(t *testing.T) {
	in := []byte(`[1,2,{"k":"v"}]`)
	out, ok := prettyJSON(in)
	if !ok {
		t.Fatalf("prettyJSON failed on valid array")
	}
	if !strings.Contains(string(out), "\n") {
		t.Errorf("array not indented: %q", out)
	}
	if compact(t, out) != compact(t, in) {
		t.Errorf("array value changed by indent")
	}
}

func TestPrettyJSON_InvalidFallsBack(t *testing.T) {
	cases := [][]byte{
		[]byte(`{not json`),
		[]byte(`<html>not json at all</html>`),
		[]byte(`{"a":}`),
		[]byte(``),
		nil,
	}
	for _, in := range cases {
		// Must not panic and must report failure so the caller shows raw.
		out, ok := prettyJSON(in)
		if ok {
			t.Errorf("prettyJSON(%q) reported success on invalid JSON (out=%q)", in, out)
		}
		// Doc contract: returns (nil, false) on invalid so caller falls back to raw.
		if out != nil {
			t.Errorf("prettyJSON(%q) returned non-nil bytes on failure: %q", in, out)
		}
	}
}

func TestPrettyJSON_IsIdempotentOnAlreadyPretty(t *testing.T) {
	in := []byte("{\n  \"a\": 1\n}")
	out, ok := prettyJSON(in)
	if !ok {
		t.Fatalf("prettyJSON failed on already-pretty JSON")
	}
	out2, ok2 := prettyJSON(out)
	if !ok2 {
		t.Fatalf("second prettyJSON pass failed")
	}
	if string(out) != string(out2) {
		t.Errorf("prettyJSON not idempotent:\n first: %q\nsecond: %q", out, out2)
	}
}

func compact(t *testing.T, b []byte) string {
	t.Helper()
	var buf bytes.Buffer
	if err := json.Compact(&buf, b); err != nil {
		t.Fatalf("compact failed on %q: %v", b, err)
	}
	return buf.String()
}
