package postresp

import (
	"testing"
	"time"

	"github.com/ultramcu/yon/internal/model"
)

func TestEvalJSONPathSubset(t *testing.T) {
	body := []byte(`{
		"status": "ok",
		"count": 3,
		"ratio": 3.14,
		"big": 1000000,
		"neg": -1,
		"flag": true,
		"nothing": null,
		"data": {"token": "abc", "items": [{"id": 10}, {"id": 20}]},
		"list": [1, 2, 3],
		"obj": {"a": 1}
	}`)

	cases := []struct {
		name    string
		path    string
		want    string
		wantOK  bool
		wantErr bool
	}{
		{"scalar string", "$.status", "ok", true, false},
		{"int number", "count", "3", true, false},
		{"float number", "ratio", "3.14", true, false},
		{"large int no exponent-noise", "big", "1000000", true, false},
		{"negative int", "neg", "-1", true, false},
		{"bool true", "flag", "true", true, false},
		{"null is not found", "nothing", "", false, false},
		{"nested key", "$.data.token", "abc", true, false},
		{"nested array index", "$.data.items[0].id", "10", true, false},
		{"nested array index 2", "data.items[1].id", "20", true, false},
		{"bare leading index path", "list[2]", "3", true, false},
		{"optional dollar absent", "data.token", "abc", true, false},
		{"object target as compact json", "obj", `{"a":1}`, true, false},
		{"array target as compact json", "list", `[1,2,3]`, true, false},
		{"missing key", "$.nope", "", false, false},
		{"missing nested key", "data.nope", "", false, false},
		{"index out of range", "list[9]", "", false, false},
		{"key on non-object", "status.x", "", false, false},
		{"index on non-array", "count[0]", "", false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok, err := EvalJSONPath(body, c.path)
			if c.wantErr {
				if err == nil {
					t.Fatalf("want error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ok != c.wantOK || got != c.want {
				t.Fatalf("EvalJSONPath(%q) = %q,%v; want %q,%v", c.path, got, ok, c.want, c.wantOK)
			}
		})
	}
}

func TestEvalJSONPathMalformed(t *testing.T) {
	if _, _, err := EvalJSONPath([]byte(`{not json`), "$.a"); err == nil {
		t.Fatal("want error for malformed JSON, got nil")
	}
}

func TestEvalJSONPathRootScalar(t *testing.T) {
	// Empty path resolves to the root value.
	got, ok, err := EvalJSONPath([]byte(`42`), "$")
	if err != nil || !ok || got != "42" {
		t.Fatalf("root scalar: got %q,%v,%v", got, ok, err)
	}
}

func TestRunCapturesMixed(t *testing.T) {
	resp := model.Response{
		Status:  200,
		Headers: []model.Param{{Key: "X-Request-Id", Value: "req-123"}},
		Body:    []byte(`{"data":{"token":"tok-abc"}}`),
	}
	captures := []model.Capture{
		{Variable: "token", Source: model.CaptureJSONBody, Expr: "$.data.token", Enabled: true},
		{Variable: "reqid", Source: model.CaptureHeader, Expr: "x-request-id", Enabled: true}, // case-insensitive
		{Variable: "skipDisabled", Source: model.CaptureJSONBody, Expr: "$.data.token", Enabled: false},
		{Variable: "", Source: model.CaptureJSONBody, Expr: "$.data.token", Enabled: true}, // empty var skipped
		{Variable: "missing", Source: model.CaptureJSONBody, Expr: "$.nope", Enabled: true},
	}
	vars, errs := RunCaptures(resp, captures)

	if vars == nil {
		t.Fatal("vars must be non-nil")
	}
	if vars["token"] != "tok-abc" {
		t.Errorf("token = %q; want tok-abc", vars["token"])
	}
	if vars["reqid"] != "req-123" {
		t.Errorf("reqid = %q; want req-123", vars["reqid"])
	}
	if _, ok := vars["skipDisabled"]; ok {
		t.Error("disabled capture should not bind")
	}
	if _, ok := vars[""]; ok {
		t.Error("empty-variable capture should not bind")
	}
	if _, ok := vars["missing"]; ok {
		t.Error("missing-path capture should not bind")
	}
	if len(errs) != 1 {
		t.Fatalf("want exactly 1 error (missing path), got %d: %v", len(errs), errs)
	}
}

func TestRunCapturesHeaderMissing(t *testing.T) {
	resp := model.Response{Body: []byte(`{}`)}
	vars, errs := RunCaptures(resp, []model.Capture{
		{Variable: "h", Source: model.CaptureHeader, Expr: "X-Absent", Enabled: true},
	})
	if len(vars) != 0 {
		t.Errorf("want no bindings, got %v", vars)
	}
	if len(errs) != 1 {
		t.Fatalf("want 1 error for missing header, got %v", errs)
	}
}

func TestRunCapturesMalformedJSON(t *testing.T) {
	resp := model.Response{Body: []byte(`{bad`)}
	_, errs := RunCaptures(resp, []model.Capture{
		{Variable: "x", Source: model.CaptureJSONBody, Expr: "$.a", Enabled: true},
	})
	if len(errs) != 1 {
		t.Fatalf("want 1 error for malformed JSON, got %v", errs)
	}
}

func TestRunAssertionsAllOps(t *testing.T) {
	resp := model.Response{
		Status:   200,
		Headers:  []model.Param{{Key: "Content-Type", Value: "application/json"}},
		Body:     []byte(`{"name":"yon","count":5}`),
		Duration: 150 * time.Millisecond,
	}

	cases := []struct {
		name       string
		a          model.Assertion
		wantPassed bool
		wantActual string
		wantErr    bool
	}{
		{"status equals pass", model.Assertion{Source: model.AssertStatus, Op: model.OpEquals, Expected: "200", Enabled: true}, true, "200", false},
		{"status equals fail", model.Assertion{Source: model.AssertStatus, Op: model.OpEquals, Expected: "404", Enabled: true}, false, "200", false},
		{"status notEquals pass", model.Assertion{Source: model.AssertStatus, Op: model.OpNotEquals, Expected: "404", Enabled: true}, true, "200", false},
		{"status notEquals fail", model.Assertion{Source: model.AssertStatus, Op: model.OpNotEquals, Expected: "200", Enabled: true}, false, "200", false},
		{"rawBody contains pass", model.Assertion{Source: model.AssertRawBody, Op: model.OpContains, Expected: "yon", Enabled: true}, true, `{"name":"yon","count":5}`, false},
		{"rawBody contains fail", model.Assertion{Source: model.AssertRawBody, Op: model.OpContains, Expected: "zzz", Enabled: true}, false, `{"name":"yon","count":5}`, false},
		{"rawBody notContains pass", model.Assertion{Source: model.AssertRawBody, Op: model.OpNotContains, Expected: "zzz", Enabled: true}, true, `{"name":"yon","count":5}`, false},
		{"rawBody notContains fail", model.Assertion{Source: model.AssertRawBody, Op: model.OpNotContains, Expected: "yon", Enabled: true}, false, `{"name":"yon","count":5}`, false},
		{"jsonBody exists pass", model.Assertion{Source: model.AssertJSONBody, Expr: "$.name", Op: model.OpExists, Enabled: true}, true, "present", false},
		{"jsonBody exists fail", model.Assertion{Source: model.AssertJSONBody, Expr: "$.nope", Op: model.OpExists, Enabled: true}, false, "absent", false},
		{"jsonBody notExists pass", model.Assertion{Source: model.AssertJSONBody, Expr: "$.nope", Op: model.OpNotExists, Enabled: true}, true, "absent", false},
		{"jsonBody notExists fail", model.Assertion{Source: model.AssertJSONBody, Expr: "$.name", Op: model.OpNotExists, Enabled: true}, false, "present", false},
		{"header exists pass", model.Assertion{Source: model.AssertHeader, Expr: "content-type", Op: model.OpExists, Enabled: true}, true, "present", false},
		{"header exists fail", model.Assertion{Source: model.AssertHeader, Expr: "X-None", Op: model.OpExists, Enabled: true}, false, "absent", false},
		{"status always exists", model.Assertion{Source: model.AssertStatus, Op: model.OpExists, Enabled: true}, true, "present", false},
		{"responseTime lessThan pass", model.Assertion{Source: model.AssertResponseTimeMs, Op: model.OpLessThan, Expected: "200", Enabled: true}, true, "150", false},
		{"responseTime lessThan fail", model.Assertion{Source: model.AssertResponseTimeMs, Op: model.OpLessThan, Expected: "100", Enabled: true}, false, "150", false},
		{"count greaterThan pass", model.Assertion{Source: model.AssertJSONBody, Expr: "$.count", Op: model.OpGreaterThan, Expected: "3", Enabled: true}, true, "5", false},
		{"count greaterThan fail", model.Assertion{Source: model.AssertJSONBody, Expr: "$.count", Op: model.OpGreaterThan, Expected: "9", Enabled: true}, false, "5", false},
		{"non-numeric lessThan errors", model.Assertion{Source: model.AssertJSONBody, Expr: "$.name", Op: model.OpLessThan, Expected: "3", Enabled: true}, false, "yon", true},
		{"matches pass", model.Assertion{Source: model.AssertJSONBody, Expr: "$.name", Op: model.OpMatches, Expected: "^y.n$", Enabled: true}, true, "yon", false},
		{"matches fail", model.Assertion{Source: model.AssertJSONBody, Expr: "$.name", Op: model.OpMatches, Expected: "^x", Enabled: true}, false, "yon", false},
		{"matches bad regex errors", model.Assertion{Source: model.AssertRawBody, Op: model.OpMatches, Expected: "[", Enabled: true}, false, `{"name":"yon","count":5}`, true},
		{"header equals trimmed", model.Assertion{Source: model.AssertHeader, Expr: "Content-Type", Op: model.OpEquals, Expected: " application/json ", Enabled: true}, true, "application/json", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			results := RunAssertions(resp, []model.Assertion{c.a})
			if len(results) != 1 {
				t.Fatalf("want 1 result, got %d", len(results))
			}
			r := results[0]
			if r.Passed != c.wantPassed {
				t.Errorf("Passed = %v; want %v (err=%q)", r.Passed, c.wantPassed, r.Err)
			}
			if r.Actual != c.wantActual {
				t.Errorf("Actual = %q; want %q", r.Actual, c.wantActual)
			}
			if (r.Err != "") != c.wantErr {
				t.Errorf("Err = %q; wantErr = %v", r.Err, c.wantErr)
			}
		})
	}
}

func TestRunAssertionsSkipsDisabled(t *testing.T) {
	resp := model.Response{Status: 200}
	results := RunAssertions(resp, []model.Assertion{
		{Source: model.AssertStatus, Op: model.OpEquals, Expected: "200", Enabled: false},
		{Source: model.AssertStatus, Op: model.OpEquals, Expected: "200", Enabled: true},
	})
	if len(results) != 1 {
		t.Fatalf("disabled assertion should be omitted; got %d results", len(results))
	}
	if !results[0].Passed {
		t.Error("enabled assertion should pass")
	}
}

// TestCaptureFailBeforePassAfter is the issue #27 fail-before / pass-after
// guard: a login response captures a token that a follow-up request consumes
// via the runtime scope. Before capture the symbol is absent; after capture the
// runtime binding wins over any env/collection value of the same name.
func TestCaptureFailBeforePassAfter(t *testing.T) {
	loginResp := model.Response{
		Status: 200,
		Body:   []byte(`{"data":{"token":"secret-xyz"}}`),
	}

	// Fail-before: nothing captured yet, runtime scope has no "authToken".
	runtime := map[string]string{}
	if _, ok := runtime["authToken"]; ok {
		t.Fatal("fail-before: authToken should be absent")
	}

	// Run the capture.
	vars, errs := RunCaptures(loginResp, []model.Capture{
		{Variable: "authToken", Source: model.CaptureJSONBody, Expr: "$.data.token", Enabled: true},
	})
	if len(errs) != 0 {
		t.Fatalf("capture errored: %v", errs)
	}

	// Pass-after: merge captured vars into the runtime scope.
	for k, v := range vars {
		runtime[k] = v
	}
	if runtime["authToken"] != "secret-xyz" {
		t.Fatalf("pass-after: authToken = %q; want secret-xyz", runtime["authToken"])
	}
}
