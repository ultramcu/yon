package yonner

import (
	"strings"
	"testing"
	"time"

	"github.com/ultramcu/yon/internal/model"
)

// TestFromCurl_Table exercises the tokenizer, flag parsing, and mapping rules.
// Before FromCurl existed these all failed (the symbol was absent → build
// error); after the implementation they pass.
func TestFromCurl_Table(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr bool
		check   func(t *testing.T, r model.Request)
	}{
		{
			name: "simple GET defaults",
			in:   "curl https://api.example.com/x",
			check: func(t *testing.T, r model.Request) {
				wantMethod(t, r, model.MethodGet)
				wantURL(t, r, "https://api.example.com/x")
				if r.Auth.Kind != model.AuthNone {
					t.Fatalf("auth = %v, want none", r.Auth.Kind)
				}
				if r.Body.Type != model.BodyNone {
					t.Fatalf("body = %v, want none", r.Body.Type)
				}
				if r.FolderID != "" || r.Name != "" {
					t.Fatalf("FolderID/Name must be empty, got %q/%q", r.FolderID, r.Name)
				}
			},
		},
		{
			name: "data implies POST",
			in:   `curl https://x/y -d 'hello=1'`,
			check: func(t *testing.T, r model.Request) {
				wantMethod(t, r, model.MethodPost)
				if r.Body.Type != model.BodyText || r.Body.Content != "hello=1" {
					t.Fatalf("body = %+v, want text hello=1", r.Body)
				}
			},
		},
		{
			name: "explicit method beats data POST default",
			in:   `curl -X PUT https://x/y -d 'a=b'`,
			check: func(t *testing.T, r model.Request) {
				wantMethod(t, r, model.MethodPut)
			},
		},
		{
			name: "JSON content-type drops header and sets body type",
			in:   `curl https://x -H 'Content-Type: application/json' --data-raw '{"a":1}'`,
			check: func(t *testing.T, r model.Request) {
				if r.Body.Type != model.BodyJSON {
					t.Fatalf("body type = %v, want json", r.Body.Type)
				}
				if hasHeader(r, "Content-Type") {
					t.Fatalf("Content-Type header should be dropped: %+v", r.Headers)
				}
			},
		},
		{
			name: "xml content-type drops header",
			in:   `curl https://x -H 'Content-Type: text/xml' -d '<a/>'`,
			check: func(t *testing.T, r model.Request) {
				if r.Body.Type != model.BodyXML {
					t.Fatalf("body type = %v, want xml", r.Body.Type)
				}
				if hasHeader(r, "Content-Type") {
					t.Fatalf("Content-Type header should be dropped")
				}
			},
		},
		{
			name: "other content-type kept, body stays text",
			in:   `curl https://x -H 'Content-Type: text/csv' -d 'a,b'`,
			check: func(t *testing.T, r model.Request) {
				if r.Body.Type != model.BodyText {
					t.Fatalf("body type = %v, want text", r.Body.Type)
				}
				if !hasHeader(r, "Content-Type") {
					t.Fatalf("custom Content-Type must be kept")
				}
			},
		},
		{
			name: "bearer header becomes auth and is dropped",
			in:   `curl https://x -H 'Authorization: Bearer tok123'`,
			check: func(t *testing.T, r model.Request) {
				if r.Auth.Kind != model.AuthBearer || r.Auth.Token != "tok123" {
					t.Fatalf("auth = %+v, want bearer tok123", r.Auth)
				}
				if hasHeader(r, "Authorization") {
					t.Fatalf("Authorization header should be dropped")
				}
			},
		},
		{
			name: "non-bearer authorization header kept",
			in:   `curl https://x -H 'Authorization: Custom abc'`,
			check: func(t *testing.T, r model.Request) {
				if r.Auth.Kind != model.AuthNone {
					t.Fatalf("auth = %v, want none", r.Auth.Kind)
				}
				if !hasHeader(r, "Authorization") {
					t.Fatalf("custom Authorization must be kept")
				}
			},
		},
		{
			name: "basic auth via -u, beats bearer header",
			in:   `curl https://x -u 'alice:secret' -H 'Authorization: Bearer tok'`,
			check: func(t *testing.T, r model.Request) {
				if r.Auth.Kind != model.AuthBasic || r.Auth.Username != "alice" || r.Auth.Password != "secret" {
					t.Fatalf("auth = %+v, want basic alice/secret", r.Auth)
				}
				if !hasHeader(r, "Authorization") {
					t.Fatalf("Authorization header kept when -u wins")
				}
			},
		},
		{
			name: "user without colon is all username",
			in:   `curl https://x -u admin`,
			check: func(t *testing.T, r model.Request) {
				if r.Auth.Username != "admin" || r.Auth.Password != "" {
					t.Fatalf("auth = %+v, want username only", r.Auth)
				}
			},
		},
		{
			name: "-I means HEAD",
			in:   `curl -I https://x`,
			check: func(t *testing.T, r model.Request) {
				wantMethod(t, r, model.MethodHead)
			},
		},
		{
			name: "shorthand A/e/b become headers",
			in:   `curl https://x -A 'agent/1' -e 'http://ref' -b 'k=v'`,
			check: func(t *testing.T, r model.Request) {
				wantHeader(t, r, "User-Agent", "agent/1")
				wantHeader(t, r, "Referer", "http://ref")
				wantHeader(t, r, "Cookie", "k=v")
			},
		},
		{
			name: "-G turns data into query params",
			in:   `curl -G https://x/y -d 'a=1' -d 'b=2'`,
			check: func(t *testing.T, r model.Request) {
				wantMethod(t, r, model.MethodGet)
				if r.Body.Type != model.BodyNone {
					t.Fatalf("body = %v, want none with -G", r.Body.Type)
				}
				if len(r.Params) != 2 || r.Params[0].Key != "a" || r.Params[1].Value != "2" {
					t.Fatalf("params = %+v, want a=1 b=2", r.Params)
				}
				if !r.Params[0].Enabled {
					t.Fatalf("query params must be enabled")
				}
			},
		},
		{
			name: "data-urlencode keeps name, encodes value",
			in:   `curl https://x --data-urlencode 'q=a b&c'`,
			check: func(t *testing.T, r model.Request) {
				if r.Body.Content != "q=a+b%26c" {
					t.Fatalf("body = %q, want q=a+b%%26c", r.Body.Content)
				}
			},
		},
		{
			name: "multiple data joined with &",
			in:   `curl https://x -d a=1 -d b=2`,
			check: func(t *testing.T, r model.Request) {
				if r.Body.Content != "a=1&b=2" {
					t.Fatalf("body = %q, want a=1&b=2", r.Body.Content)
				}
			},
		},
		{
			name: "tolerated flags ignored, combined short -sL",
			in:   `curl -sL --compressed --max-time 10 -o out.txt https://x`,
			check: func(t *testing.T, r model.Request) {
				wantURL(t, r, "https://x")
				wantMethod(t, r, model.MethodGet)
			},
		},
		{
			name: "templates kept intact in URL",
			in:   `curl '{{base}}/users?id={{id}}'`,
			check: func(t *testing.T, r model.Request) {
				wantURL(t, r, "{{base}}/users?id={{id}}")
			},
		},
		{
			name: "line continuations joined",
			in:   "curl https://x \\\n  -H 'X-A: 1' \\\n  -d 'k=v'",
			check: func(t *testing.T, r model.Request) {
				wantURL(t, r, "https://x")
				wantHeader(t, r, "X-A", "1")
				if r.Body.Content != "k=v" {
					t.Fatalf("body = %q", r.Body.Content)
				}
			},
		},
		{
			name: "ANSI-C quoting decodes escapes",
			in:   `curl https://x --data-raw $'line1\nline2'`,
			check: func(t *testing.T, r model.Request) {
				if r.Body.Content != "line1\nline2" {
					t.Fatalf("body = %q, want line1<nl>line2", r.Body.Content)
				}
			},
		},
		{
			name: "adjacent runs concatenate into one token",
			in:   `curl https://x -H'X-Y: z'`,
			check: func(t *testing.T, r model.Request) {
				wantHeader(t, r, "X-Y", "z")
			},
		},
		{
			name: "header with empty value via semicolon",
			in:   `curl https://x -H 'X-Empty;'`,
			check: func(t *testing.T, r model.Request) {
				wantHeader(t, r, "X-Empty", "")
			},
		},
		{
			name: "@file data kept literal, not read",
			in:   `curl https://x -d @payload.json`,
			check: func(t *testing.T, r model.Request) {
				if r.Body.Content != "@payload.json" {
					t.Fatalf("body = %q, want literal @payload.json", r.Body.Content)
				}
			},
		},
		{
			name:    "empty input errors",
			in:      "   ",
			wantErr: true,
		},
		{
			name:    "no URL errors",
			in:      "curl -X POST",
			wantErr: true,
		},
		{
			name:    "unterminated quote errors",
			in:      `curl 'https://x`,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := FromCurl(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got req %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.check != nil {
				tc.check(t, got)
			}
		})
	}
}

// TestFromCurl_RoundTrip pins the inverse relationship with ToCurl: for several
// representative Requests, FromCurl(ToCurl(req, …)) reproduces the meaningful
// fields. This is the core contract — paste-edit fidelity.
func TestFromCurl_RoundTrip(t *testing.T) {
	opts := Options{Timeout: 30 * time.Second, FollowRedirects: true, InsecureTLS: true}

	reqs := []model.Request{
		{
			Method: model.MethodGet,
			URL:    "https://api.example.com/items",
		},
		{
			Method:  model.MethodPost,
			URL:     "https://api.example.com/users",
			Headers: []model.Param{{Key: "X-Demo", Value: "yon", Enabled: true}},
			Auth:    model.Auth{Kind: model.AuthBearer, Token: "tok"},
			Body:    model.Body{Type: model.BodyJSON, Content: `{"a":1,"b":"x y"}`},
		},
		{
			Method: model.MethodPut,
			URL:    "https://h/p",
			Auth:   model.Auth{Kind: model.AuthBasic, Username: "al ice", Password: "p'wd"},
			Body:   model.Body{Type: model.BodyText, Content: "plain text body"},
		},
		{
			Method:  model.MethodDelete,
			URL:     "https://h/p",
			Headers: []model.Param{{Key: "X-Token", Value: "abc", Enabled: true}},
		},
		// NB: BodyXML is intentionally not round-tripped here: ToCurl emits a
		// Content-Type header only for BodyJSON (curl.go), so an XML body's
		// type is not recoverable from the curl text and comes back as text.
		// The one-way XML parse (Content-Type: text/xml → BodyXML) is covered
		// in TestFromCurl_Table instead.
	}

	for _, want := range reqs {
		t.Run(string(want.Method)+" "+want.URL, func(t *testing.T) {
			curl := ToCurl(want, model.Collection{}, opts)
			got, err := FromCurl(curl)
			if err != nil {
				t.Fatalf("FromCurl(%q) error: %v", curl, err)
			}

			if got.Method != want.Method {
				t.Errorf("method = %q, want %q (curl: %s)", got.Method, want.Method, curl)
			}
			// URL may gain enabled params folded into the query by ToCurl, but
			// these requests have none, so it must match exactly.
			if got.URL != want.URL {
				t.Errorf("url = %q, want %q", got.URL, want.URL)
			}
			// A literal Request with no body has Body.Type "" (the zero value);
			// FromCurl normalises that to BodyNone, so compare normalised.
			wantBodyType := want.Body.Type
			if wantBodyType == "" {
				wantBodyType = model.BodyNone
			}
			if got.Body.Type != wantBodyType || got.Body.Content != want.Body.Content {
				t.Errorf("body = %+v, want {Type:%s Content:%q}", got.Body, wantBodyType, want.Body.Content)
			}
			if got.Auth.Kind != wantAuthKind(want.Auth) {
				t.Errorf("auth kind = %q, want %q", got.Auth.Kind, wantAuthKind(want.Auth))
			}
			switch want.Auth.Kind {
			case model.AuthBearer:
				if got.Auth.Token != want.Auth.Token {
					t.Errorf("token = %q, want %q", got.Auth.Token, want.Auth.Token)
				}
			case model.AuthBasic:
				if got.Auth.Username != want.Auth.Username || got.Auth.Password != want.Auth.Password {
					t.Errorf("basic = %q/%q, want %q/%q", got.Auth.Username, got.Auth.Password, want.Auth.Username, want.Auth.Password)
				}
			}
			// Every original enabled header should survive (those that aren't
			// auth/content-type special-cases).
			for _, h := range want.Headers {
				if !hasHeader(got, h.Key) {
					t.Errorf("missing header %q after round-trip; headers=%+v", h.Key, got.Headers)
				}
			}
		})
	}
}

// wantAuthKind maps an empty Auth.Kind (an unset Auth on a literal Request) to
// AuthNone, which is what FromCurl produces.
func wantAuthKind(a model.Auth) model.AuthKind {
	if a.Kind == "" {
		return model.AuthNone
	}
	return a.Kind
}

func wantMethod(t *testing.T, r model.Request, m model.Method) {
	t.Helper()
	if r.Method != m {
		t.Fatalf("method = %q, want %q", r.Method, m)
	}
}

func wantURL(t *testing.T, r model.Request, u string) {
	t.Helper()
	if r.URL != u {
		t.Fatalf("url = %q, want %q", r.URL, u)
	}
}

func hasHeader(r model.Request, key string) bool {
	for _, h := range r.Headers {
		if strings.EqualFold(h.Key, key) {
			return true
		}
	}
	return false
}

func wantHeader(t *testing.T, r model.Request, key, val string) {
	t.Helper()
	for _, h := range r.Headers {
		if strings.EqualFold(h.Key, key) {
			if h.Value != val {
				t.Fatalf("header %q = %q, want %q", key, h.Value, val)
			}
			if !h.Enabled {
				t.Fatalf("header %q must be enabled", key)
			}
			return
		}
	}
	t.Fatalf("header %q not found in %+v", key, r.Headers)
}
