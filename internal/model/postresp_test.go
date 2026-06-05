package model

import (
	"encoding/json"
	"testing"
)

// TestRequestUnsetCapturesAssertionsOmitted verifies a Request without Captures
// or Assertions serializes byte-identically to before those fields existed: the
// "captures" and "assertions" keys must be absent entirely (backward compat).
func TestRequestUnsetCapturesAssertionsOmitted(t *testing.T) {
	req := Request{
		Method: MethodGet,
		URL:    "https://example.com",
		Auth:   Auth{Kind: AuthNone},
		Body:   Body{Type: BodyNone},
	}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)

	// This is the exact JSON a pre-feature build produced for the same Request.
	const want = `{"method":"GET","url":"https://example.com","auth":{"kind":"none"},"body":{"type":"none"}}`
	if got != want {
		t.Fatalf("unset captures/assertions changed JSON:\n got: %s\nwant: %s", got, want)
	}
}

// TestRequestCapturesAssertionsRoundTrip checks captures/assertions survive a
// marshal/unmarshal cycle and that, once set, they appear in the JSON.
func TestRequestCapturesAssertionsRoundTrip(t *testing.T) {
	req := Request{
		Method: MethodPost,
		URL:    "https://api.example.com/login",
		Auth:   Auth{Kind: AuthNone},
		Body:   Body{Type: BodyJSON, Content: `{"u":"a"}`},
		Captures: []Capture{
			{Variable: "token", Source: CaptureJSONBody, Expr: "$.data.token", Enabled: true},
			{Variable: "reqID", Source: CaptureHeader, Expr: "X-Request-Id", Enabled: false},
		},
		Assertions: []Assertion{
			{Source: AssertStatus, Op: OpEquals, Expected: "200", Enabled: true},
			{Source: AssertJSONBody, Expr: "$.ok", Op: OpExists, Enabled: true},
		},
	}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got Request
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Captures) != 2 || got.Captures[0].Variable != "token" ||
		got.Captures[0].Source != CaptureJSONBody || got.Captures[0].Expr != "$.data.token" ||
		!got.Captures[0].Enabled || got.Captures[1].Enabled {
		t.Fatalf("captures round-trip mismatch: %+v", got.Captures)
	}
	if len(got.Assertions) != 2 || got.Assertions[0].Op != OpEquals ||
		got.Assertions[0].Expected != "200" || got.Assertions[1].Op != OpExists {
		t.Fatalf("assertions round-trip mismatch: %+v", got.Assertions)
	}
}
