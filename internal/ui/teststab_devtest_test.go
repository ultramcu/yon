package ui

import (
	"testing"

	"github.com/ultramcu/yon/internal/model"
)

// These tests cover the PURE, Fyne-free row→model mapping helpers and the
// label↔value maps in teststab.go. No widgets are constructed, so they run
// without a Fyne app driver.

func TestCaptureSourceLabelRoundTrip(t *testing.T) {
	for _, s := range []model.CaptureSource{model.CaptureJSONBody, model.CaptureHeader} {
		if got := captureSourceFromLabel(captureSourceToLabel(s)); got != s {
			t.Errorf("CaptureSource round-trip %q: got %q", s, got)
		}
	}
	// Unknown/empty label defaults to jsonBody.
	if got := captureSourceFromLabel(""); got != model.CaptureJSONBody {
		t.Errorf("empty label: got %q, want jsonBody", got)
	}
	if got := captureSourceFromLabel("nonsense"); got != model.CaptureJSONBody {
		t.Errorf("unknown label: got %q, want jsonBody", got)
	}
}

func TestAssertSourceLabelRoundTrip(t *testing.T) {
	for _, s := range []model.AssertSource{
		model.AssertStatus, model.AssertJSONBody, model.AssertHeader,
		model.AssertResponseTimeMs, model.AssertRawBody,
	} {
		if got := assertSourceFromLabel(assertSourceToLabel(s)); got != s {
			t.Errorf("AssertSource round-trip %q: got %q", s, got)
		}
	}
	if got := assertSourceFromLabel(""); got != model.AssertStatus {
		t.Errorf("empty label: got %q, want status", got)
	}
}

func TestAssertOpLabelRoundTrip(t *testing.T) {
	for _, op := range []model.AssertOp{
		model.OpEquals, model.OpNotEquals, model.OpContains, model.OpNotContains,
		model.OpExists, model.OpNotExists, model.OpLessThan, model.OpGreaterThan,
		model.OpMatches,
	} {
		if got := assertOpFromLabel(assertOpToLabel(op)); got != op {
			t.Errorf("AssertOp round-trip %q: got %q", op, got)
		}
	}
	if got := assertOpFromLabel(""); got != model.OpEquals {
		t.Errorf("empty label: got %q, want equals", got)
	}
}

func TestCaptureFromRow(t *testing.T) {
	got := captureFromRow("token", captureSourceHeaderLabel, "X-Token", true)
	want := model.Capture{
		Variable: "token",
		Source:   model.CaptureHeader,
		Expr:     "X-Token",
		Enabled:  true,
	}
	if got != want {
		t.Errorf("captureFromRow = %+v, want %+v", got, want)
	}

	// JSON-body source, disabled.
	got = captureFromRow("id", captureSourceJSONBodyLabel, "data.id", false)
	if got.Source != model.CaptureJSONBody || got.Enabled {
		t.Errorf("captureFromRow jsonBody/disabled = %+v", got)
	}
}

func TestAssertionFromRow(t *testing.T) {
	got := assertionFromRow(assertSourceStatusLabel, "", assertOpEqualsLabel, "200", true)
	want := model.Assertion{
		Source:   model.AssertStatus,
		Expr:     "",
		Op:       model.OpEquals,
		Expected: "200",
		Enabled:  true,
	}
	if got != want {
		t.Errorf("assertionFromRow = %+v, want %+v", got, want)
	}

	// jsonBody + contains, disabled.
	got = assertionFromRow(assertSourceJSONBodyLabel, "data.name", assertOpContainsLabel, "ada", false)
	if got.Source != model.AssertJSONBody || got.Op != model.OpContains ||
		got.Expr != "data.name" || got.Expected != "ada" || got.Enabled {
		t.Errorf("assertionFromRow jsonBody/contains = %+v", got)
	}
}

func TestCaptureRowEmpty(t *testing.T) {
	if !captureRowEmpty("", "") {
		t.Error("blank row should be empty")
	}
	if captureRowEmpty("token", "") {
		t.Error("row with variable should not be empty")
	}
	if captureRowEmpty("", "data.id") {
		t.Error("row with expr should not be empty")
	}
}
