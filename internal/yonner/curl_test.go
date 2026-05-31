package yonner

import (
	"strings"
	"testing"
	"time"

	"github.com/ultramcu/yon/internal/model"
)

func TestToCurl_FullRequest(t *testing.T) {
	req := model.Request{
		Method:  model.MethodPost,
		URL:     "https://api.example.com/users?team=x",
		Params:  []model.Param{{Key: "page", Value: "1", Enabled: true}, {Key: "off", Value: "y"}},
		Headers: []model.Param{{Key: "X-Demo", Value: "yon", Enabled: true}},
		Auth:    model.Auth{Kind: model.AuthBearer, Token: "tok"},
		Body:    model.Body{Type: model.BodyJSON, Content: `{"a":1}`},
	}
	opts := Options{Timeout: 30 * time.Second, FollowRedirects: true, InsecureTLS: true}
	got := ToCurl(req, model.Collection{}, opts)

	for _, want := range []string{
		"curl -L -k --max-time 30",
		"-X POST",
		"'https://api.example.com/users?team=x&page=1'", // existing query kept + enabled param; disabled omitted
		"-H 'X-Demo: yon'",
		"-H 'Authorization: Bearer tok'",
		"-H 'Content-Type: application/json'",
		`--data-raw '{"a":1}'`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}

func TestToCurl_BasicAuthQuotingAndGet(t *testing.T) {
	req := model.Request{
		Method: model.MethodGet,
		URL:    "http://x/y",
		Auth:   model.Auth{Kind: model.AuthBasic, Username: "al ice", Password: "p'wd"},
	}
	got := ToCurl(req, model.Collection{}, Options{})

	if !strings.Contains(got, `-u 'al ice:p'\''wd'`) {
		t.Fatalf("basic-auth shell quoting wrong:\n%s", got)
	}
	if strings.Contains(got, "-X GET") {
		t.Fatalf("GET must not emit -X:\n%s", got)
	}
}

func TestToCurl_ExplicitAuthHeaderWins(t *testing.T) {
	req := model.Request{
		Method:  model.MethodGet,
		URL:     "http://x/y",
		Headers: []model.Param{{Key: "Authorization", Value: "Token abc", Enabled: true}},
		Auth:    model.Auth{Kind: model.AuthBearer, Token: "tok"},
	}
	got := ToCurl(req, model.Collection{}, Options{})
	if !strings.Contains(got, "-H 'Authorization: Token abc'") || strings.Contains(got, "Bearer tok") {
		t.Fatalf("explicit Authorization header should win:\n%s", got)
	}
}
