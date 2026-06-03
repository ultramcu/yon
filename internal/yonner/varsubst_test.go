package yonner_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ultramcu/yon/internal/model"
	"github.com/ultramcu/yon/internal/yonner"
)

// resolveFunc is the test substitution map used across the cases below. It maps
// the {{var}} placeholders the tests embed in a Request to their resolved
// values, leaving any other string untouched (identity for unknown input).
//
// {{base}} resolves to the live test server URL so a Request can reference the
// server symbolically; the remaining vars are plain literals.
func resolveFunc(serverURL string) func(string) string {
	repl := strings.NewReplacer(
		"{{base}}", serverURL,
		"{{path}}", "users",
		"{{q}}", "hello",
		"{{hdr}}", "X1",
		"{{tok}}", "secrettoken",
	)
	return func(s string) string {
		return repl.Replace(s)
	}
}

// ---------------------------------------------------------------------------
// 1. URL / path substitution.
// ---------------------------------------------------------------------------

// A Request.URL of "{{base}}/{{path}}" with a Resolve func mapping {{base}} to
// the server URL and {{path}} to "users" must reach the server at path
// "/users" — proving Resolve is applied to the whole URL (scheme+host+path)
// before the HTTP request is built.
func TestVarSubst_URLPathResolved(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
	}))
	defer srv.Close()

	req := model.Request{
		Method: model.MethodGet,
		URL:    "{{base}}/{{path}}",
	}
	opts := yonner.DefaultOptions()
	opts.Resolve = resolveFunc(srv.URL)

	if _, err := yonner.Send(context.Background(), req, model.NewCollection("c"), opts); err != nil {
		t.Fatalf("Send error: %v", err)
	}
	if gotPath != "/users" {
		t.Errorf("server path: got %q, want %q", gotPath, "/users")
	}
}

// ---------------------------------------------------------------------------
// 2. Query param value substitution.
// ---------------------------------------------------------------------------

// An enabled query Param {key:"q", value:"{{q}}"} must reach the server as
// q=="hello" — proving Resolve is applied to enabled query param values.
func TestVarSubst_QueryParamValueResolved(t *testing.T) {
	var gotQ string
	var present bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQ = r.URL.Query().Get("q")
		_, present = r.URL.Query()["q"]
	}))
	defer srv.Close()

	req := model.Request{
		Method: model.MethodGet,
		URL:    "{{base}}",
		Params: []model.Param{
			{Key: "q", Value: "{{q}}", Enabled: true},
		},
	}
	opts := yonner.DefaultOptions()
	opts.Resolve = resolveFunc(srv.URL)

	if _, err := yonner.Send(context.Background(), req, model.NewCollection("c"), opts); err != nil {
		t.Fatalf("Send error: %v", err)
	}
	if !present {
		t.Fatalf("query param q missing from request")
	}
	if gotQ != "hello" {
		t.Errorf("server query q: got %q, want %q", gotQ, "hello")
	}
}

// ---------------------------------------------------------------------------
// 3. Header value substitution.
// ---------------------------------------------------------------------------

// An enabled header {key:"X-Tag", value:"{{hdr}}"} must reach the server as
// X-Tag=="X1" — proving Resolve is applied to enabled header values.
func TestVarSubst_HeaderValueResolved(t *testing.T) {
	var gotHdr string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHdr = r.Header.Get("X-Tag")
	}))
	defer srv.Close()

	req := model.Request{
		Method: model.MethodGet,
		URL:    "{{base}}",
		Headers: []model.Param{
			{Key: "X-Tag", Value: "{{hdr}}", Enabled: true},
		},
	}
	opts := yonner.DefaultOptions()
	opts.Resolve = resolveFunc(srv.URL)

	if _, err := yonner.Send(context.Background(), req, model.NewCollection("c"), opts); err != nil {
		t.Fatalf("Send error: %v", err)
	}
	if gotHdr != "X1" {
		t.Errorf("server header X-Tag: got %q, want %q", gotHdr, "X1")
	}
}

// ---------------------------------------------------------------------------
// 4. Body content substitution.
// ---------------------------------------------------------------------------

// A JSON body containing "{{q}}" must reach the server with "hello"
// substituted in (and NOT the literal "{{q}}") — proving Resolve is applied to
// the body content.
func TestVarSubst_BodyContentResolved(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
	}))
	defer srv.Close()

	req := model.Request{
		Method: model.MethodPost,
		URL:    "{{base}}",
		Body:   model.Body{Type: model.BodyJSON, Content: `{"greeting":"{{q}}"}`},
	}
	opts := yonner.DefaultOptions()
	opts.Resolve = resolveFunc(srv.URL)

	if _, err := yonner.Send(context.Background(), req, model.NewCollection("c"), opts); err != nil {
		t.Fatalf("Send error: %v", err)
	}
	if strings.Contains(gotBody, "{{q}}") {
		t.Errorf("body still contains literal placeholder: %q", gotBody)
	}
	if !strings.Contains(gotBody, "hello") {
		t.Errorf("body should contain resolved value %q, got %q", "hello", gotBody)
	}
	if gotBody != `{"greeting":"hello"}` {
		t.Errorf("server body: got %q, want %q", gotBody, `{"greeting":"hello"}`)
	}
}

// ---------------------------------------------------------------------------
// 5. Auth (Bearer token) substitution.
// ---------------------------------------------------------------------------

// A bearer Auth with Token "{{tok}}" must reach the server as
// Authorization=="Bearer secrettoken" — proving Resolve is applied to the auth
// Token before the Authorization header is derived.
func TestVarSubst_AuthBearerTokenResolved(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
	}))
	defer srv.Close()

	req := model.Request{
		Method: model.MethodGet,
		URL:    "{{base}}",
		Auth:   model.Auth{Kind: model.AuthBearer, Token: "{{tok}}"},
	}
	opts := yonner.DefaultOptions()
	opts.Resolve = resolveFunc(srv.URL)

	if _, err := yonner.Send(context.Background(), req, model.NewCollection("c"), opts); err != nil {
		t.Fatalf("Send error: %v", err)
	}
	if gotAuth != "Bearer secrettoken" {
		t.Errorf("server Authorization: got %q, want %q", gotAuth, "Bearer secrettoken")
	}
}

// ---------------------------------------------------------------------------
// 6. Identity default — Resolve == nil leaves placeholders literal.
// ---------------------------------------------------------------------------

// With Options.Resolve == nil, no substitution happens: a {{var}} placeholder
// in a query value is sent verbatim (URL-encoded by net/url) rather than
// expanded. This proves existing behaviour is unchanged when Resolve is unset.
//
// The Request targets a fixed server (no {{base}} in the URL) and carries the
// literal placeholder only in a query value, so the request still reaches the
// server and the literal is observable in the raw query.
func TestVarSubst_NilResolveLeavesPlaceholderLiteral(t *testing.T) {
	var gotRawQuery, gotQVal string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRawQuery = r.URL.RawQuery
		gotQVal = r.URL.Query().Get("q")
	}))
	defer srv.Close()

	req := model.Request{
		Method: model.MethodGet,
		URL:    srv.URL,
		Params: []model.Param{
			{Key: "q", Value: "{{base}}", Enabled: true},
		},
	}
	opts := yonner.DefaultOptions()
	opts.Resolve = nil // identity / no substitution

	if _, err := yonner.Send(context.Background(), req, model.NewCollection("c"), opts); err != nil {
		t.Fatalf("Send error: %v", err)
	}

	// The decoded query value must remain the literal placeholder, NOT expanded
	// to the server URL or anything else.
	if gotQVal != "{{base}}" {
		t.Errorf("q value with nil Resolve: got %q, want literal %q", gotQVal, "{{base}}")
	}

	// Raw (encoded) query must show the percent-encoded braces and must NOT
	// contain the server URL the placeholder would have resolved to.
	if !strings.Contains(gotRawQuery, "%7B%7Bbase%7D%7D") && !strings.Contains(gotRawQuery, "{{base}}") {
		t.Errorf("raw query should carry the literal placeholder, got %q", gotRawQuery)
	}
	if strings.Contains(gotRawQuery, "http") {
		t.Errorf("nil Resolve must not expand {{base}}; raw query unexpectedly contains a URL: %q", gotRawQuery)
	}
}

// DefaultOptions must leave Resolve nil (identity) so the zero-config send path
// is unchanged — a guard that the new field defaults to "no substitution".
func TestVarSubst_DefaultOptionsResolveIsNil(t *testing.T) {
	if yonner.DefaultOptions().Resolve != nil {
		t.Errorf("DefaultOptions().Resolve should default to nil (identity)")
	}
}
