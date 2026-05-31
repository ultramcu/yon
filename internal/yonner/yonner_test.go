package yonner_test

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ultramcu/yon/internal/model"
	"github.com/ultramcu/yon/internal/yonner"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func headerParam(key, value string) model.Param {
	return model.Param{Key: key, Value: value, Enabled: true}
}

// findHeader returns the value of the named header from a model.Response's
// Param slice (case-insensitive key match), and whether it was present.
func findRespHeader(resp model.Response, key string) (string, bool) {
	for _, p := range resp.Headers {
		if strings.EqualFold(p.Key, key) {
			return p.Value, true
		}
	}
	return "", false
}

// ===========================================================================
// Build tests — derive expectations purely from SCOPE/CONTEXT.
// ===========================================================================

// Enabled Params are appended to the URL and PRESERVE an existing query
// string already present in the URL (concatenate, not overwrite). Disabled
// params are omitted.
func TestBuild_ParamsAppendedPreserveExistingQuery(t *testing.T) {
	req := model.Request{
		Method: model.MethodGet,
		URL:    "http://example.com/path?existing=1",
		Params: []model.Param{
			{Key: "a", Value: "x", Enabled: true},
			{Key: "b", Value: "y", Enabled: false}, // disabled -> omitted
			{Key: "c", Value: "z z", Enabled: true},
		},
	}
	coll := model.NewCollection("c")

	hr, err := yonner.Build(context.Background(), req, coll)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	q := hr.URL.Query()

	// Existing query string preserved.
	if got := q.Get("existing"); got != "1" {
		t.Errorf("existing query param: got %q, want %q", got, "1")
	}
	// Enabled params appended.
	if got := q.Get("a"); got != "x" {
		t.Errorf("param a: got %q, want %q", got, "x")
	}
	if got := q.Get("c"); got != "z z" {
		t.Errorf("param c: got %q, want %q", got, "z z")
	}
	// Disabled param omitted.
	if _, ok := q["b"]; ok {
		t.Errorf("disabled param b should be omitted, but present: %v", q["b"])
	}
}

// Enabled Headers are set; disabled headers omitted.
func TestBuild_HeadersEnabledSetDisabledOmitted(t *testing.T) {
	req := model.Request{
		Method: model.MethodGet,
		URL:    "http://example.com/",
		Headers: []model.Param{
			{Key: "X-Enabled", Value: "yes", Enabled: true},
			{Key: "X-Disabled", Value: "no", Enabled: false},
		},
	}
	hr, err := yonner.Build(context.Background(), req, model.NewCollection("c"))
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if got := hr.Header.Get("X-Enabled"); got != "yes" {
		t.Errorf("X-Enabled: got %q, want %q", got, "yes")
	}
	if got := hr.Header.Get("X-Disabled"); got != "" {
		t.Errorf("X-Disabled should be omitted, got %q", got)
	}
}

// Auth basic -> Authorization: Basic base64(user:pass).
func TestBuild_AuthBasic(t *testing.T) {
	req := model.Request{
		Method: model.MethodGet,
		URL:    "http://example.com/",
		Auth:   model.Auth{Kind: model.AuthBasic, Username: "user", Password: "pass"},
	}
	hr, err := yonner.Build(context.Background(), req, model.NewCollection("c"))
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("user:pass"))
	if got := hr.Header.Get("Authorization"); got != want {
		t.Errorf("Authorization basic: got %q, want %q", got, want)
	}
}

// Auth bearer -> Authorization: Bearer <token>.
func TestBuild_AuthBearer(t *testing.T) {
	req := model.Request{
		Method: model.MethodGet,
		URL:    "http://example.com/",
		Auth:   model.Auth{Kind: model.AuthBearer, Token: "tok123"},
	}
	hr, err := yonner.Build(context.Background(), req, model.NewCollection("c"))
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if got := hr.Header.Get("Authorization"); got != "Bearer tok123" {
		t.Errorf("Authorization bearer: got %q, want %q", got, "Bearer tok123")
	}
}

// Auth inherit pulls from the Collection.
func TestBuild_AuthInheritFromCollection(t *testing.T) {
	req := model.Request{
		Method: model.MethodGet,
		URL:    "http://example.com/",
		Auth:   model.Auth{Kind: model.AuthInherit},
	}
	coll := model.Collection{
		Version: 1,
		Name:    "c",
		Auth:    model.Auth{Kind: model.AuthBearer, Token: "colltok"},
	}
	hr, err := yonner.Build(context.Background(), req, coll)
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if got := hr.Header.Get("Authorization"); got != "Bearer colltok" {
		t.Errorf("inherited Authorization: got %q, want %q", got, "Bearer colltok")
	}
}

// Request Auth "none" overrides Collection auth -> no Authorization derived.
func TestBuild_AuthNoneOverridesCollection(t *testing.T) {
	req := model.Request{
		Method: model.MethodGet,
		URL:    "http://example.com/",
		Auth:   model.Auth{Kind: model.AuthNone},
	}
	coll := model.Collection{
		Version: 1,
		Name:    "c",
		Auth:    model.Auth{Kind: model.AuthBearer, Token: "colltok"},
	}
	hr, err := yonner.Build(context.Background(), req, coll)
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if got := hr.Header.Get("Authorization"); got != "" {
		t.Errorf("AuthNone should produce no Authorization, got %q", got)
	}
}

// Explicit Authorization header in the table WINS over derived auth.
func TestBuild_ExplicitAuthorizationHeaderWins(t *testing.T) {
	req := model.Request{
		Method: model.MethodGet,
		URL:    "http://example.com/",
		Headers: []model.Param{
			headerParam("Authorization", "Custom abc"),
		},
		Auth: model.Auth{Kind: model.AuthBearer, Token: "tok123"},
	}
	hr, err := yonner.Build(context.Background(), req, model.NewCollection("c"))
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if got := hr.Header.Get("Authorization"); got != "Custom abc" {
		t.Errorf("explicit Authorization should win: got %q, want %q", got, "Custom abc")
	}
}

// Body JSON: auto Content-Type only if the user didn't set one.
func TestBuild_JSONBodyAutoContentType(t *testing.T) {
	req := model.Request{
		Method: model.MethodPost,
		URL:    "http://example.com/",
		Body:   model.Body{Type: model.BodyJSON, Content: `{"k":"v"}`},
	}
	hr, err := yonner.Build(context.Background(), req, model.NewCollection("c"))
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if got := hr.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("auto Content-Type: got %q, want %q", got, "application/json")
	}
	// Body present.
	b, _ := io.ReadAll(hr.Body)
	if string(b) != `{"k":"v"}` {
		t.Errorf("body: got %q, want %q", string(b), `{"k":"v"}`)
	}
}

// Body JSON: user-set Content-Type is NOT overwritten.
func TestBuild_JSONBodyUserContentTypeKept(t *testing.T) {
	req := model.Request{
		Method: model.MethodPost,
		URL:    "http://example.com/",
		Headers: []model.Param{
			headerParam("Content-Type", "application/vnd.api+json"),
		},
		Body: model.Body{Type: model.BodyJSON, Content: `{"k":"v"}`},
	}
	hr, err := yonner.Build(context.Background(), req, model.NewCollection("c"))
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if got := hr.Header.Get("Content-Type"); got != "application/vnd.api+json" {
		t.Errorf("user Content-Type should be kept: got %q, want %q", got, "application/vnd.api+json")
	}
}

// Body sent for ANY method including GET when non-empty.
func TestBuild_BodySentOnGET(t *testing.T) {
	req := model.Request{
		Method: model.MethodGet,
		URL:    "http://example.com/",
		Body:   model.Body{Type: model.BodyText, Content: "hello"},
	}
	hr, err := yonner.Build(context.Background(), req, model.NewCollection("c"))
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if hr.Body == nil {
		t.Fatalf("GET body should be present when non-empty, got nil")
	}
	b, _ := io.ReadAll(hr.Body)
	if string(b) != "hello" {
		t.Errorf("GET body: got %q, want %q", string(b), "hello")
	}
}

// Text body: no automatic Content-Type.
func TestBuild_TextBodyNoAutoContentType(t *testing.T) {
	req := model.Request{
		Method: model.MethodPost,
		URL:    "http://example.com/",
		Body:   model.Body{Type: model.BodyText, Content: "raw text"},
	}
	hr, err := yonner.Build(context.Background(), req, model.NewCollection("c"))
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if got := hr.Header.Get("Content-Type"); got != "" {
		t.Errorf("text body should not auto-set Content-Type, got %q", got)
	}
}

// Correct method recorded in the built request.
func TestBuild_MethodSet(t *testing.T) {
	for _, m := range []model.Method{model.MethodGet, model.MethodPost, model.MethodPut, model.MethodDelete} {
		req := model.Request{Method: m, URL: "http://example.com/"}
		hr, err := yonner.Build(context.Background(), req, model.NewCollection("c"))
		if err != nil {
			t.Fatalf("Build error for %s: %v", m, err)
		}
		if hr.Method != string(m) {
			t.Errorf("method: got %q, want %q", hr.Method, string(m))
		}
	}
}

// ===========================================================================
// Send tests — against httptest.Server, no real network.
// ===========================================================================

// Correct method reaches the server; basic auth header arrives; body arrives;
// model.Response fields are populated.
func TestSend_MethodAuthBodyAndResponseFields(t *testing.T) {
	var gotMethod, gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("X-Custom", "val")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, "response-body")
	}))
	defer srv.Close()

	req := model.Request{
		Method: model.MethodPost,
		URL:    srv.URL,
		Auth:   model.Auth{Kind: model.AuthBasic, Username: "u", Password: "p"},
		Body:   model.Body{Type: model.BodyText, Content: "request-body"},
	}

	resp, err := yonner.Send(context.Background(), req, model.NewCollection("c"), yonner.DefaultOptions())
	if err != nil {
		t.Fatalf("Send error: %v", err)
	}

	// Server-side assertions.
	if gotMethod != "POST" {
		t.Errorf("server method: got %q, want POST", gotMethod)
	}
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("u:p"))
	if gotAuth != wantAuth {
		t.Errorf("server Authorization: got %q, want %q", gotAuth, wantAuth)
	}
	if gotBody != "request-body" {
		t.Errorf("server body: got %q, want %q", gotBody, "request-body")
	}

	// Response field assertions.
	if resp.Status != http.StatusCreated {
		t.Errorf("resp.Status: got %d, want %d", resp.Status, http.StatusCreated)
	}
	if string(resp.Body) != "response-body" {
		t.Errorf("resp.Body: got %q, want %q", string(resp.Body), "response-body")
	}
	if resp.Size != int64(len("response-body")) {
		t.Errorf("resp.Size: got %d, want %d", resp.Size, len("response-body"))
	}
	if resp.Duration <= 0 {
		t.Errorf("resp.Duration should be > 0, got %v", resp.Duration)
	}
	if v, ok := findRespHeader(resp, "X-Custom"); !ok || v != "val" {
		t.Errorf("resp header X-Custom: got %q present=%v, want %q", v, ok, "val")
	}
}

// Bearer auth header arrives at the server.
func TestSend_BearerAuthArrives(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
	}))
	defer srv.Close()

	req := model.Request{
		Method: model.MethodGet,
		URL:    srv.URL,
		Auth:   model.Auth{Kind: model.AuthBearer, Token: "abc.def"},
	}
	if _, err := yonner.Send(context.Background(), req, model.NewCollection("c"), yonner.DefaultOptions()); err != nil {
		t.Fatalf("Send error: %v", err)
	}
	if gotAuth != "Bearer abc.def" {
		t.Errorf("server Authorization: got %q, want %q", gotAuth, "Bearer abc.def")
	}
}

// Enabled query params reach the server, disabled omitted, existing preserved.
func TestSend_ParamsReachServer(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
	}))
	defer srv.Close()

	req := model.Request{
		Method: model.MethodGet,
		URL:    srv.URL + "/?keep=1",
		Params: []model.Param{
			{Key: "on", Value: "1", Enabled: true},
			{Key: "off", Value: "1", Enabled: false},
		},
	}
	if _, err := yonner.Send(context.Background(), req, model.NewCollection("c"), yonner.DefaultOptions()); err != nil {
		t.Fatalf("Send error: %v", err)
	}
	if !strings.Contains(gotQuery, "keep=1") {
		t.Errorf("existing query not preserved: %q", gotQuery)
	}
	if !strings.Contains(gotQuery, "on=1") {
		t.Errorf("enabled param missing: %q", gotQuery)
	}
	if strings.Contains(gotQuery, "off=1") {
		t.Errorf("disabled param should be omitted: %q", gotQuery)
	}
}

// DefaultOptions (30s-ish timeout) works for a normal request.
func TestSend_DefaultOptionsWorks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	defer srv.Close()

	req := model.Request{Method: model.MethodGet, URL: srv.URL}
	resp, err := yonner.Send(context.Background(), req, model.NewCollection("c"), yonner.DefaultOptions())
	if err != nil {
		t.Fatalf("Send error: %v", err)
	}
	if resp.Status != http.StatusOK {
		t.Errorf("status: got %d, want 200", resp.Status)
	}
	if string(resp.Body) != "ok" {
		t.Errorf("body: got %q, want ok", string(resp.Body))
	}
}

// InsecureTLS=true allows a request to a self-signed TLS server to succeed.
func TestSend_InsecureTLSTrueSucceeds(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "secure-ok")
	}))
	defer srv.Close()

	req := model.Request{Method: model.MethodGet, URL: srv.URL}
	opts := yonner.DefaultOptions()
	opts.InsecureTLS = true

	resp, err := yonner.Send(context.Background(), req, model.NewCollection("c"), opts)
	if err != nil {
		t.Fatalf("Send with InsecureTLS=true should succeed, got error: %v", err)
	}
	if string(resp.Body) != "secure-ok" {
		t.Errorf("body: got %q, want secure-ok", string(resp.Body))
	}
}

// InsecureTLS=false fails with a certificate error against a self-signed server.
func TestSend_InsecureTLSFalseFailsCertError(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "secure-ok")
	}))
	defer srv.Close()

	req := model.Request{Method: model.MethodGet, URL: srv.URL}
	opts := yonner.DefaultOptions()
	opts.InsecureTLS = false

	_, err := yonner.Send(context.Background(), req, model.NewCollection("c"), opts)
	if err == nil {
		t.Fatalf("Send with InsecureTLS=false against self-signed server should fail with a cert error, got nil")
	}
}

// FollowRedirects=false returns the 3xx response rather than following it.
func TestSend_FollowRedirectsFalseReturnsRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/dest", http.StatusFound)
			return
		}
		// destination — should NOT be reached when not following.
		_, _ = io.WriteString(w, "destination")
	}))
	defer srv.Close()

	req := model.Request{Method: model.MethodGet, URL: srv.URL + "/start"}
	opts := yonner.DefaultOptions()
	opts.FollowRedirects = false

	resp, err := yonner.Send(context.Background(), req, model.NewCollection("c"), opts)
	if err != nil {
		t.Fatalf("Send (no-follow) should return the 3xx, not error: %v", err)
	}
	if resp.Status != http.StatusFound {
		t.Errorf("status: got %d, want %d (302)", resp.Status, http.StatusFound)
	}
	if loc, ok := findRespHeader(resp, "Location"); !ok || loc != "/dest" {
		t.Errorf("Location header: got %q present=%v, want %q", loc, ok, "/dest")
	}
	if strings.Contains(string(resp.Body), "destination") {
		t.Errorf("should not have followed redirect; body=%q", string(resp.Body))
	}
}

// FollowRedirects=true (default) follows the redirect to its destination.
func TestSend_FollowRedirectsTrueFollows(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/dest", http.StatusFound)
			return
		}
		_, _ = io.WriteString(w, "destination")
	}))
	defer srv.Close()

	req := model.Request{Method: model.MethodGet, URL: srv.URL + "/start"}
	resp, err := yonner.Send(context.Background(), req, model.NewCollection("c"), yonner.DefaultOptions())
	if err != nil {
		t.Fatalf("Send error: %v", err)
	}
	if resp.Status != http.StatusOK {
		t.Errorf("status after follow: got %d, want 200", resp.Status)
	}
	if string(resp.Body) != "destination" {
		t.Errorf("body after follow: got %q, want destination", string(resp.Body))
	}
}

// Context cancellation returns an error.
func TestSend_ContextCancelReturnsError(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block // hang until the test releases it
	}))
	defer srv.Close()
	defer close(block)

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel shortly after Send starts.
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	req := model.Request{Method: model.MethodGet, URL: srv.URL}
	_, err := yonner.Send(ctx, req, model.NewCollection("c"), yonner.DefaultOptions())
	if err == nil {
		t.Fatalf("Send should error on context cancel, got nil")
	}
}

// Context timeout returns an error.
func TestSend_ContextTimeoutReturnsError(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
	}))
	defer srv.Close()
	defer close(block)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	req := model.Request{Method: model.MethodGet, URL: srv.URL}
	_, err := yonner.Send(ctx, req, model.NewCollection("c"), yonner.DefaultOptions())
	if err == nil {
		t.Fatalf("Send should error on context timeout, got nil")
	}
}
