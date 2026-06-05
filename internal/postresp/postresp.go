// Package postresp runs Yon's post-response logic: extracting values from a
// Response into runtime variables (captures) and checking the Response against
// declared expectations (assertions).
//
// The engine is pure and deterministic: it depends only on the standard library
// and Yon's model package, and imports neither Fyne nor any networking package.
// It includes a small dependency-free JSON path evaluator (EvalJSONPath) rather
// than pulling in a third-party JSONPath library.
package postresp

import (
	"encoding/json"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/ultramcu/yon/internal/model"
)

// EvalJSONPath evaluates a small JSON-path subset against body (parsed as JSON)
// and returns the value at path as a string.
//
// Path grammar (a deliberate subset, no third-party dependency):
//   - a leading "$" is optional ("$.a.b" and "a.b" are equivalent);
//   - segments are split on ".";
//   - a numeric index uses "[N]" and may follow a key or another index
//     (e.g. "$.data.items[0].id", "a.b", "items[2]", "[0]").
//
// The path is resolved into the parsed value. Result conversion:
//   - a string scalar yields the string itself;
//   - a number yields its shortest exact decimal form (an integer prints without
//     a trailing ".0", e.g. 200, -1, 3.14);
//   - a bool yields "true"/"false";
//   - JSON null yields ok=false (treated as "not found");
//   - an object or array target yields its compact json.Marshal form, ok=true.
//
// A path that does not resolve — a missing key, an index out of range, or an
// index/key applied to the wrong kind of value — yields ok=false with err=nil
// (a clean "not found"). Malformed JSON in body yields err != nil.
func EvalJSONPath(body []byte, path string) (value string, ok bool, err error) {
	var root interface{}
	if err := json.Unmarshal(body, &root); err != nil {
		return "", false, err
	}

	segs, perr := parsePath(path)
	if perr != nil {
		return "", false, perr
	}

	cur := root
	for _, seg := range segs {
		if seg.index >= 0 {
			arr, isArr := cur.([]interface{})
			if !isArr || seg.index >= len(arr) {
				return "", false, nil
			}
			cur = arr[seg.index]
			continue
		}
		obj, isObj := cur.(map[string]interface{})
		if !isObj {
			return "", false, nil
		}
		next, found := obj[seg.key]
		if !found {
			return "", false, nil
		}
		cur = next
	}

	return scalarString(cur)
}

// segment is one step of a parsed path: either an object key (index < 0) or an
// array index (index >= 0, key empty).
type segment struct {
	key   string
	index int
}

// parsePath turns a path string into ordered segments. A leading "$" is dropped.
// "[N]" segments produce index segments; everything else is a key segment. An
// empty key (e.g. a stray "..") is rejected, as is a malformed "[...]".
func parsePath(path string) ([]segment, error) {
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, "$")
	path = strings.TrimPrefix(path, ".")
	if path == "" {
		return nil, nil
	}

	var segs []segment
	for _, part := range strings.Split(path, ".") {
		if part == "" {
			return nil, &pathError{path: path, msg: "empty path segment"}
		}
		// A part may carry a key followed by one or more "[N]" indices, or be a
		// bare "[N]" index. Walk the brackets off the end after the key.
		key := part
		if br := strings.IndexByte(part, '['); br >= 0 {
			key = part[:br]
			rest := part[br:]
			if key != "" {
				segs = append(segs, segment{key: key, index: -1})
			}
			idxSegs, err := parseIndices(rest, path)
			if err != nil {
				return nil, err
			}
			segs = append(segs, idxSegs...)
			continue
		}
		segs = append(segs, segment{key: key, index: -1})
	}
	return segs, nil
}

// parseIndices parses a run of "[N]" tokens (e.g. "[0][2]") into index segments.
func parseIndices(s, path string) ([]segment, error) {
	var segs []segment
	for s != "" {
		if s[0] != '[' {
			return nil, &pathError{path: path, msg: "expected '[' in index"}
		}
		end := strings.IndexByte(s, ']')
		if end < 0 {
			return nil, &pathError{path: path, msg: "unterminated index"}
		}
		n, err := strconv.Atoi(s[1:end])
		if err != nil || n < 0 {
			return nil, &pathError{path: path, msg: "invalid index " + s[1:end]}
		}
		segs = append(segs, segment{index: n})
		s = s[end+1:]
	}
	return segs, nil
}

// pathError reports a malformed path expression.
type pathError struct {
	path string
	msg  string
}

func (e *pathError) Error() string {
	return "postresp: bad json path " + strconv.Quote(e.path) + ": " + e.msg
}

// scalarString renders a resolved JSON value as a string per EvalJSONPath's
// rules. null returns ok=false; objects and arrays return their compact JSON.
func scalarString(v interface{}) (string, bool, error) {
	switch t := v.(type) {
	case nil:
		return "", false, nil
	case string:
		return t, true, nil
	case bool:
		if t {
			return "true", true, nil
		}
		return "false", true, nil
	case float64:
		// An integral value prints in plain decimal (no "1e+06" exponent noise
		// and no trailing ".0"); a fractional value uses the shortest exact form.
		if t == math.Trunc(t) && !math.IsInf(t, 0) && math.Abs(t) < 1e21 {
			return strconv.FormatFloat(t, 'f', -1, 64), true, nil
		}
		return strconv.FormatFloat(t, 'g', -1, 64), true, nil
	default: // object or array
		b, err := json.Marshal(t)
		if err != nil {
			return "", false, err
		}
		return string(b), true, nil
	}
}

// RunCaptures runs each ENABLED capture against resp and returns the extracted
// variable bindings plus any errors. For a jsonBody capture the value comes from
// EvalJSONPath(resp.Body, Expr); for a header capture it is the value of the
// first response header whose Key equals Expr case-insensitively. A capture with
// an empty Variable is skipped. A capture that does not resolve (missing path,
// absent header, or malformed JSON) contributes a descriptive error and no
// binding. The returned vars map is always non-nil.
func RunCaptures(resp model.Response, captures []model.Capture) (vars map[string]string, errs []error) {
	vars = make(map[string]string)
	for _, c := range captures {
		if !c.Enabled || c.Variable == "" {
			continue
		}
		switch c.Source {
		case model.CaptureJSONBody:
			val, ok, err := EvalJSONPath(resp.Body, c.Expr)
			if err != nil {
				errs = append(errs, &captureError{variable: c.Variable, msg: "json body is not valid JSON: " + err.Error()})
				continue
			}
			if !ok {
				errs = append(errs, &captureError{variable: c.Variable, msg: "no value at json path " + strconv.Quote(c.Expr)})
				continue
			}
			vars[c.Variable] = val
		case model.CaptureHeader:
			val, ok := headerValue(resp.Headers, c.Expr)
			if !ok {
				errs = append(errs, &captureError{variable: c.Variable, msg: "no response header " + strconv.Quote(c.Expr)})
				continue
			}
			vars[c.Variable] = val
		default:
			errs = append(errs, &captureError{variable: c.Variable, msg: "unknown capture source " + strconv.Quote(string(c.Source))})
		}
	}
	return vars, errs
}

// captureError describes a single capture that failed to extract a value.
type captureError struct {
	variable string
	msg      string
}

func (e *captureError) Error() string {
	return "capture " + strconv.Quote(e.variable) + ": " + e.msg
}

// AssertionResult is the outcome of running one Assertion: the Assertion itself,
// whether it Passed, the Actual value that was compared, and a human-readable
// Err describing why it could not be evaluated (empty when there was no error).
type AssertionResult struct {
	Assertion model.Assertion
	Passed    bool
	Actual    string
	Err       string
}

// RunAssertions runs each ENABLED assertion against resp and returns one
// AssertionResult per enabled assertion (disabled assertions are omitted). For
// each it derives the actual value from the source, then applies the operator.
// A comparison that cannot be evaluated (e.g. a non-numeric lessThan/greaterThan
// operand or a bad regexp) yields Passed=false with Err set.
func RunAssertions(resp model.Response, assertions []model.Assertion) []AssertionResult {
	var results []AssertionResult
	for _, a := range assertions {
		if !a.Enabled {
			continue
		}
		results = append(results, evalAssertion(resp, a))
	}
	return results
}

// evalAssertion computes the actual value for one assertion and applies its Op.
func evalAssertion(resp model.Response, a model.Assertion) AssertionResult {
	res := AssertionResult{Assertion: a}

	// actual is the derived string value; present reports whether the source/
	// path exists at all (used by exists/notExists).
	var actual string
	present := true
	switch a.Source {
	case model.AssertStatus:
		actual = strconv.Itoa(resp.Status)
	case model.AssertResponseTimeMs:
		actual = strconv.FormatInt(resp.Duration.Milliseconds(), 10)
	case model.AssertRawBody:
		actual = string(resp.Body)
	case model.AssertJSONBody:
		val, ok, err := EvalJSONPath(resp.Body, a.Expr)
		if err != nil {
			res.Err = "json body is not valid JSON: " + err.Error()
			res.Passed = false
			return res
		}
		actual = val
		present = ok
	case model.AssertHeader:
		val, ok := headerValue(resp.Headers, a.Expr)
		actual = val // "" when absent
		present = ok
	default:
		res.Err = "unknown assertion source " + strconv.Quote(string(a.Source))
		res.Passed = false
		return res
	}

	res.Actual = actual
	applyOp(&res, a, actual, present)
	return res
}

// applyOp applies the assertion operator to the derived actual value, setting
// Passed/Err (and, for exists/notExists, a descriptive Actual note).
func applyOp(res *AssertionResult, a model.Assertion, actual string, present bool) {
	switch a.Op {
	case model.OpEquals:
		res.Passed = strings.TrimSpace(actual) == strings.TrimSpace(a.Expected)
	case model.OpNotEquals:
		res.Passed = strings.TrimSpace(actual) != strings.TrimSpace(a.Expected)
	case model.OpContains:
		res.Passed = strings.Contains(actual, a.Expected)
	case model.OpNotContains:
		res.Passed = !strings.Contains(actual, a.Expected)
	case model.OpExists:
		res.Passed = present
		res.Actual = existsNote(present)
	case model.OpNotExists:
		res.Passed = !present
		res.Actual = existsNote(present)
	case model.OpLessThan, model.OpGreaterThan:
		applyNumeric(res, a, actual)
	case model.OpMatches:
		ok, err := regexp.MatchString(a.Expected, actual)
		if err != nil {
			res.Passed = false
			res.Err = "invalid regular expression: " + err.Error()
			return
		}
		res.Passed = ok
	default:
		res.Passed = false
		res.Err = "unknown assertion operator " + strconv.Quote(string(a.Op))
	}
}

// applyNumeric handles the numeric comparison operators, parsing both operands
// as float64 and setting Err when either is non-numeric.
func applyNumeric(res *AssertionResult, a model.Assertion, actual string) {
	av, aerr := strconv.ParseFloat(strings.TrimSpace(actual), 64)
	ev, eerr := strconv.ParseFloat(strings.TrimSpace(a.Expected), 64)
	if aerr != nil {
		res.Passed = false
		res.Err = "actual value " + strconv.Quote(actual) + " is not numeric"
		return
	}
	if eerr != nil {
		res.Passed = false
		res.Err = "expected value " + strconv.Quote(a.Expected) + " is not numeric"
		return
	}
	if a.Op == model.OpLessThan {
		res.Passed = av < ev
	} else {
		res.Passed = av > ev
	}
}

// existsNote returns the Actual placeholder used for exists/notExists results.
func existsNote(present bool) string {
	if present {
		return "present"
	}
	return "absent"
}

// headerValue returns the value of the first header whose Key equals name
// case-insensitively, and whether such a header was found.
func headerValue(headers []model.Param, name string) (string, bool) {
	for _, h := range headers {
		if strings.EqualFold(h.Key, name) {
			return h.Value, true
		}
	}
	return "", false
}
