package postresp

// Blind tests for issue #27 — capture + assertions engine. Written from the
// contract only; the implementation is intentionally not consulted. All symbols
// referenced here come from the package contract (EvalJSONPath, RunCaptures,
// RunAssertions, AssertionResult) and from internal/model.

import (
	"strings"
	"testing"
	"time"

	"github.com/ultramcu/yon/internal/model"
)

// sampleBody is the nested JSON fixture shared by the EvalJSONPath, assertion,
// and capture tests.
const sampleBody = `{"data":{"items":[{"id":7,"name":"a"},{"id":8}],"ok":true,"ratio":1.5,"nothing":null}}`

// TestEvalJSONPath pins the JSON path subset: leading $ optional, dotted
// segments, [N] numeric index; scalar formatting (number minimal, bool
// true/false), null => ok=false, object/array => compact JSON with ok=true,
// missing path/out-of-range index => ok=false err=nil, malformed body => err.
func TestBlindEvalJSONPath(t *testing.T) {
	t.Parallel()

	body := []byte(sampleBody)

	// scalar / index / dotted cases that must succeed with an exact string.
	scalarCases := []struct {
		name string
		path string
		want string
	}{
		{"index then field, leading $", "$.data.items[0].id", "7"},
		{"index then field, no leading $", "data.items[1].id", "8"},
		{"bool true", "$.data.ok", "true"},
		{"number with fraction", "$.data.ratio", "1.5"},
		{"string field", "$.data.items[0].name", "a"},
	}
	for _, tc := range scalarCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok, err := EvalJSONPath(body, tc.path)
			if err != nil {
				t.Fatalf("EvalJSONPath(%q) unexpected err: %v", tc.path, err)
			}
			if !ok {
				t.Fatalf("EvalJSONPath(%q) ok=false, want true", tc.path)
			}
			if got != tc.want {
				t.Fatalf("EvalJSONPath(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}

	// missing / out-of-range / null => ok=false, err=nil.
	notFoundCases := []struct {
		name string
		path string
	}{
		{"missing field", "$.data.nope"},
		{"out-of-range index", "$.data.items[5].id"},
		{"null value", "$.data.nothing"},
	}
	for _, tc := range notFoundCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok, err := EvalJSONPath(body, tc.path)
			if err != nil {
				t.Fatalf("EvalJSONPath(%q) unexpected err: %v", tc.path, err)
			}
			if ok {
				t.Fatalf("EvalJSONPath(%q) ok=true (val %q), want false", tc.path, got)
			}
		})
	}

	// object => ok=true and value is its compact JSON (must contain "items").
	t.Run("object yields compact json", func(t *testing.T) {
		t.Parallel()
		got, ok, err := EvalJSONPath(body, "$.data")
		if err != nil {
			t.Fatalf("EvalJSONPath($.data) unexpected err: %v", err)
		}
		if !ok {
			t.Fatalf("EvalJSONPath($.data) ok=false, want true")
		}
		if !strings.Contains(got, `"items"`) {
			t.Fatalf("EvalJSONPath($.data) = %q, want JSON containing %q", got, `"items"`)
		}
	})

	// malformed body => err != nil.
	t.Run("malformed body errors", func(t *testing.T) {
		t.Parallel()
		_, _, err := EvalJSONPath([]byte(`{not json`), "$.data")
		if err == nil {
			t.Fatalf("EvalJSONPath(malformed) err=nil, want non-nil")
		}
	})
}

// newResponse builds the shared Response fixture for assertions/captures.
func newResponse() model.Response {
	return model.Response{
		Status:   200,
		Duration: 150 * time.Millisecond,
		Headers: []model.Param{
			{Key: "Content-Type", Value: "application/json", Enabled: true},
		},
		Body: []byte(sampleBody),
	}
}

// TestRunAssertions pins per-op evaluation over each source, case-insensitive
// header names, bad-regex error reporting, and that disabled assertions are
// omitted from the results.
func TestBlindRunAssertions(t *testing.T) {
	t.Parallel()

	resp := newResponse()

	type want struct {
		passed     bool
		actual     string // checked only when non-empty
		errPresent bool
	}
	cases := []struct {
		name string
		a    model.Assertion
		want want
	}{
		{
			name: "status equals 200 passes",
			a:    model.Assertion{Source: model.AssertStatus, Op: model.OpEquals, Expected: "200", Enabled: true},
			want: want{passed: true, actual: "200"},
		},
		{
			name: "status equals 404 fails with actual 200",
			a:    model.Assertion{Source: model.AssertStatus, Op: model.OpEquals, Expected: "404", Enabled: true},
			want: want{passed: false, actual: "200"},
		},
		{
			name: "jsonBody id equals 7 passes",
			a:    model.Assertion{Source: model.AssertJSONBody, Expr: "$.data.items[0].id", Op: model.OpEquals, Expected: "7", Enabled: true},
			want: want{passed: true, actual: "7"},
		},
		{
			name: "header contains json, case-insensitive name",
			a:    model.Assertion{Source: model.AssertHeader, Expr: "content-type", Op: model.OpContains, Expected: "json", Enabled: true},
			want: want{passed: true},
		},
		{
			name: "responseTimeMs lessThan 1000 passes",
			a:    model.Assertion{Source: model.AssertResponseTimeMs, Op: model.OpLessThan, Expected: "1000", Enabled: true},
			want: want{passed: true},
		},
		{
			name: "responseTimeMs greaterThan 1000 fails",
			a:    model.Assertion{Source: model.AssertResponseTimeMs, Op: model.OpGreaterThan, Expected: "1000", Enabled: true},
			want: want{passed: false},
		},
		{
			name: "jsonBody ok exists passes",
			a:    model.Assertion{Source: model.AssertJSONBody, Expr: "$.data.ok", Op: model.OpExists, Enabled: true},
			want: want{passed: true},
		},
		{
			name: "jsonBody nope exists fails",
			a:    model.Assertion{Source: model.AssertJSONBody, Expr: "$.data.nope", Op: model.OpExists, Enabled: true},
			want: want{passed: false},
		},
		{
			name: "rawBody contains items passes",
			a:    model.Assertion{Source: model.AssertRawBody, Op: model.OpContains, Expected: "items", Enabled: true},
			want: want{passed: true},
		},
		{
			name: "status matches 2xx regex passes",
			a:    model.Assertion{Source: model.AssertStatus, Op: model.OpMatches, Expected: "^2..$", Enabled: true},
			want: want{passed: true},
		},
		{
			name: "bad regex sets Err and fails",
			a:    model.Assertion{Source: model.AssertStatus, Op: model.OpMatches, Expected: "(", Enabled: true},
			want: want{passed: false, errPresent: true},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			results := RunAssertions(resp, []model.Assertion{tc.a})
			if len(results) != 1 {
				t.Fatalf("RunAssertions returned %d results, want 1", len(results))
			}
			got := results[0]
			if got.Passed != tc.want.passed {
				t.Fatalf("Passed = %v, want %v (Actual=%q Err=%q)", got.Passed, tc.want.passed, got.Actual, got.Err)
			}
			if tc.want.actual != "" && got.Actual != tc.want.actual {
				t.Fatalf("Actual = %q, want %q", got.Actual, tc.want.actual)
			}
			if tc.want.errPresent && got.Err == "" {
				t.Fatalf("Err = empty, want non-empty for %q", tc.name)
			}
			if !tc.want.errPresent && got.Err != "" {
				t.Fatalf("Err = %q, want empty for %q", got.Err, tc.name)
			}
			// The Assertion carried in the result must be the one we passed in.
			if got.Assertion != tc.a {
				t.Fatalf("result.Assertion = %+v, want %+v", got.Assertion, tc.a)
			}
		})
	}

	t.Run("disabled assertion omitted", func(t *testing.T) {
		t.Parallel()
		results := RunAssertions(resp, []model.Assertion{
			{Source: model.AssertStatus, Op: model.OpEquals, Expected: "200", Enabled: false},
			{Source: model.AssertStatus, Op: model.OpEquals, Expected: "200", Enabled: true},
		})
		if len(results) != 1 {
			t.Fatalf("RunAssertions returned %d results, want 1 (disabled omitted)", len(results))
		}
		if !results[0].Passed {
			t.Fatalf("the single enabled assertion should pass; got %+v", results[0])
		}
	})
}

// TestRunCaptures pins jsonBody and header captures, missing-path error
// reporting, and that disabled/empty-Variable captures are skipped. The
// returned map is always non-nil.
func TestBlindRunCaptures(t *testing.T) {
	t.Parallel()

	resp := newResponse()

	t.Run("jsonBody and header capture", func(t *testing.T) {
		t.Parallel()
		vars, errs := RunCaptures(resp, []model.Capture{
			{Variable: "id", Source: model.CaptureJSONBody, Expr: "$.data.items[0].id", Enabled: true},
			{Variable: "ct", Source: model.CaptureHeader, Expr: "Content-Type", Enabled: true},
		})
		if len(errs) != 0 {
			t.Fatalf("RunCaptures errs = %v, want none", errs)
		}
		if vars == nil {
			t.Fatalf("RunCaptures map is nil, want non-nil")
		}
		if vars["id"] != "7" {
			t.Fatalf("vars[id] = %q, want %q", vars["id"], "7")
		}
		if vars["ct"] != "application/json" {
			t.Fatalf("vars[ct] = %q, want %q", vars["ct"], "application/json")
		}
	})

	t.Run("missing path appends error and omits var", func(t *testing.T) {
		t.Parallel()
		vars, errs := RunCaptures(resp, []model.Capture{
			{Variable: "gone", Source: model.CaptureJSONBody, Expr: "$.data.nope", Enabled: true},
		})
		if len(errs) == 0 {
			t.Fatalf("RunCaptures errs empty, want an error for missing path")
		}
		if _, present := vars["gone"]; present {
			t.Fatalf("vars[gone] present (%q), want absent", vars["gone"])
		}
	})

	t.Run("disabled and empty-variable skipped, map non-nil", func(t *testing.T) {
		t.Parallel()
		vars, errs := RunCaptures(resp, []model.Capture{
			{Variable: "skip", Source: model.CaptureJSONBody, Expr: "$.data.items[0].id", Enabled: false},
			{Variable: "", Source: model.CaptureJSONBody, Expr: "$.data.items[0].id", Enabled: true},
		})
		if len(errs) != 0 {
			t.Fatalf("RunCaptures errs = %v, want none", errs)
		}
		if vars == nil {
			t.Fatalf("RunCaptures map is nil, want non-nil")
		}
		if _, present := vars["skip"]; present {
			t.Fatalf("disabled capture produced vars[skip], want absent")
		}
		if len(vars) != 0 {
			t.Fatalf("vars = %v, want empty (disabled + empty-variable skipped)", vars)
		}
	})
}
