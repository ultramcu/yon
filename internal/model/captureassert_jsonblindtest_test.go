package model

// Blind test for issue #27 — Request.Captures/Assertions JSON shape. Written
// from the contract: both fields are omitempty, so a Request without them
// marshals WITHOUT the "captures"/"assertions" keys; a Request carrying some
// round-trips equal. Only contract symbols are used.

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// TestRequestCaptureAssertOmitempty pins that an empty Captures/Assertions slice
// produces no "captures"/"assertions" key in the marshalled JSON.
func TestRequestCaptureAssertOmitempty(t *testing.T) {
	t.Parallel()

	req := Request{Method: MethodGet, URL: "https://example.com"}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	js := string(b)
	if strings.Contains(js, "captures") {
		t.Fatalf("marshalled JSON contains \"captures\" key, want omitted: %s", js)
	}
	if strings.Contains(js, "assertions") {
		t.Fatalf("marshalled JSON contains \"assertions\" key, want omitted: %s", js)
	}
}

// TestRequestCaptureAssertRoundTrip pins that a Request carrying captures and
// assertions survives a marshal/unmarshal round-trip unchanged.
func TestRequestCaptureAssertRoundTrip(t *testing.T) {
	t.Parallel()

	req := Request{
		Method: MethodPost,
		URL:    "https://example.com/api",
		Captures: []Capture{
			{Variable: "id", Source: CaptureJSONBody, Expr: "$.data.id", Enabled: true},
			{Variable: "ct", Source: CaptureHeader, Expr: "Content-Type", Enabled: true},
		},
		Assertions: []Assertion{
			{Source: AssertStatus, Op: OpEquals, Expected: "200", Enabled: true},
			{Source: AssertJSONBody, Expr: "$.data.id", Op: OpExists, Enabled: true},
		},
	}

	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got Request
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(got.Captures, req.Captures) {
		t.Fatalf("Captures round-trip mismatch:\n got  %+v\n want %+v", got.Captures, req.Captures)
	}
	if !reflect.DeepEqual(got.Assertions, req.Assertions) {
		t.Fatalf("Assertions round-trip mismatch:\n got  %+v\n want %+v", got.Assertions, req.Assertions)
	}
}
