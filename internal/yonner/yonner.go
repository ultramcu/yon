// Package yonner is Yon's build-and-send engine: it turns a model.Request
// (together with its Collection, for Auth inheritance) into a real *http.Request,
// sends it, and produces a model.Response.
//
// It depends only on the standard library and internal/model. It never imports
// Fyne or any UI package (the UI-free-core rule), so it is pure, testable Go that could also
// back a future CLI.
package yonner

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ultramcu/yon/internal/model"
)

// Options controls how Send dials and runs an HTTP request. These are the
// app-level connection defaults; v1 has no per-request override.
type Options struct {
	// Timeout is the total time budget for the request (dial + send + read).
	// A zero value means no client timeout (rely on ctx instead).
	Timeout time.Duration
	// InsecureTLS, when true, disables TLS certificate verification.
	InsecureTLS bool
	// FollowRedirects, when false, makes Send return the first (redirect)
	// response instead of following it.
	FollowRedirects bool
}

// DefaultOptions returns the standard connection defaults: a 30s timeout,
// certificate verification on, and redirect following enabled.
func DefaultOptions() Options {
	return Options{
		Timeout:         30 * time.Second,
		InsecureTLS:     false,
		FollowRedirects: true,
	}
}

// headerAuthorization is the canonical Authorization header name.
const headerAuthorization = "Authorization"

// headerContentType is the canonical Content-Type header name.
const headerContentType = "Content-Type"

// Build turns a Request (plus its Collection, for Auth inheritance) into a real
// *http.Request bound to ctx. It:
//
//   - parses req.URL and appends every enabled Param in req.Params as a query
//     parameter, preserving (concatenating with) any query already present in
//     the URL — the URL query and the Param table are never merged or
//     deduplicated, matching Yon's "no two-way sync" rule;
//   - sets every enabled Header from req.Headers;
//   - resolves Auth via model.ResolveAuth and applies it as an Authorization
//     header (basic or bearer), unless the user already supplied an enabled
//     Authorization header in the table — in which case the explicit header
//     wins and Auth is not applied;
//   - sends Body as-is for any Method (including GET) when Body.Type is not
//     "none" and Body.Content is non-empty, and for JSON bodies adds
//     Content-Type: application/json (XML bodies add application/xml) only when
//     the user has not already set a Content-Type header.
func Build(ctx context.Context, req model.Request, coll model.Collection) (*http.Request, error) {
	// Build the URL with enabled query params appended in order (see requestURL,
	// shared with ToCurl).
	finalURL, err := requestURL(req)
	if err != nil {
		return nil, err
	}

	// Body: WYSIWYG — send for any method when present.
	var body io.Reader
	hasBody := req.Body.Type != model.BodyNone && req.Body.Content != ""
	if hasBody {
		body = strings.NewReader(req.Body.Content)
	}

	httpReq, err := http.NewRequestWithContext(ctx, string(req.Method), finalURL, body)
	if err != nil {
		return nil, err
	}

	// Track whether the user explicitly set Authorization / Content-Type
	// (case-insensitive) so derived values never clobber explicit ones.
	userSetAuth := false
	userSetContentType := false

	// Set enabled headers from the table. http.Header.Add canonicalizes keys,
	// so case differences collapse correctly.
	for _, h := range req.Headers {
		if !h.Enabled {
			continue
		}
		if strings.EqualFold(h.Key, headerAuthorization) {
			userSetAuth = true
		}
		if strings.EqualFold(h.Key, headerContentType) {
			userSetContentType = true
		}
		httpReq.Header.Add(h.Key, h.Value)
	}

	// Auth: derived Authorization, only when the user didn't set one explicitly.
	if !userSetAuth {
		auth := model.ResolveAuth(req, coll)
		switch auth.Kind {
		case model.AuthBasic:
			httpReq.SetBasicAuth(auth.Username, auth.Password)
		case model.AuthBearer:
			httpReq.Header.Set(headerAuthorization, "Bearer "+auth.Token)
		default:
			// "none" / "inherit"-resolved-to-none: send no Authorization.
		}
	}

	// JSON auto Content-Type, only when present and not user-overridden.
	if hasBody && req.Body.Type == model.BodyJSON && !userSetContentType {
		httpReq.Header.Set(headerContentType, "application/json")
	}

	// XML auto Content-Type, only when present and not user-overridden.
	if hasBody && req.Body.Type == model.BodyXML && !userSetContentType {
		httpReq.Header.Set(headerContentType, "application/xml")
	}

	return httpReq, nil
}

// Send builds the request via Build and sends it with an *http.Client honoring
// opts (Timeout, InsecureTLS, FollowRedirects). It measures the round-trip
// Duration, reads the full body, and returns a model.Response. The supplied ctx
// governs cancellation and deadlines; a cancelled or timed-out ctx yields a
// meaningful error rather than a partial Response.
func Send(ctx context.Context, req model.Request, coll model.Collection, opts Options) (model.Response, error) {
	httpReq, err := Build(ctx, req, coll)
	if err != nil {
		return model.Response{}, err
	}

	client := newClient(opts)

	start := time.Now()
	resp, err := client.Do(httpReq)
	if err != nil {
		return model.Response{}, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	duration := time.Since(start)
	if err != nil {
		return model.Response{}, err
	}

	return model.Response{
		Status:     resp.StatusCode,
		StatusText: statusText(resp),
		Headers:    headersToParams(resp.Header),
		Body:       bodyBytes,
		Duration:   duration,
		Size:       int64(len(bodyBytes)),
	}, nil
}

// newClient builds an *http.Client configured from opts: total Timeout,
// InsecureTLS (InsecureSkipVerify on the transport's TLS config), and
// FollowRedirects (ErrUseLastResponse to stop following when false).
func newClient(opts Options) *http.Client {
	// Clone http.DefaultTransport so requests honour the standard environment
	// proxy (HTTP_PROXY/HTTPS_PROXY/NO_PROXY), HTTP/2, and the sane connection
	// timeouts — a desktop API client must work behind a corporate proxy.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if opts.InsecureTLS {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // opt-in app-level toggle
	}

	client := &http.Client{
		Timeout:   opts.Timeout,
		Transport: transport,
	}
	if !opts.FollowRedirects {
		client.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
	return client
}

// statusText returns a human-readable status: the standard text for the code,
// falling back to the raw resp.Status line when the code is unknown.
func statusText(resp *http.Response) string {
	if t := http.StatusText(resp.StatusCode); t != "" {
		return t
	}
	return resp.Status
}

// headersToParams flattens response headers into a []model.Param with each
// entry Enabled, preserving multi-value headers as separate Params.
func headersToParams(h http.Header) []model.Param {
	var params []model.Param
	for key, values := range h {
		for _, v := range values {
			params = append(params, model.Param{Key: key, Value: v, Enabled: true})
		}
	}
	return params
}
