package yonner

import (
	"strings"
	"testing"

	"github.com/ultramcu/yon/internal/model"
)

// BT_A1 — SPEC A: with a nil/identity resolver, an unresolved {{variable}} in
// the URL path AND in an enabled query param must stay literal in the curl
// output, never percent-encoded to %7B%7B…%7D%7D.
func TestBT_Curl_UnresolvedTemplatesStayLiteral(t *testing.T) {
	req := model.Request{
		Method: model.Method("GET"),
		URL:    "{{server}}/usage",
		Params: []model.Param{
			{Key: "account", Value: "{{account}}", Enabled: true},
		},
	}
	got := ToCurl(req, model.NewCollection(""), Options{})

	if strings.Contains(got, "%7B") || strings.Contains(got, "%7b") {
		t.Errorf("curl contains a percent-encoded brace; want none.\n got: %q", got)
	}
	if !strings.Contains(got, "{{server}}/usage") {
		t.Errorf("curl missing literal path template %q.\n got: %q", "{{server}}/usage", got)
	}
	if !strings.Contains(got, "account={{account}}") {
		t.Errorf("curl missing literal param template %q.\n got: %q", "account={{account}}", got)
	}
}

// BT_A2 — SPEC A no-regression: a normal request with real values still encodes
// correctly; a param value containing a space must become %20 or + on the wire.
func TestBT_Curl_RealValueWithSpaceStillEncoded(t *testing.T) {
	req := model.Request{
		Method: model.Method("GET"),
		URL:    "https://example.com/usage",
		Params: []model.Param{
			{Key: "q", Value: "hello world", Enabled: true},
		},
	}
	got := ToCurl(req, model.NewCollection(""), Options{})

	if !strings.Contains(got, "q=hello%20world") && !strings.Contains(got, "q=hello+world") {
		t.Errorf("space not percent-encoded; want q=hello%%20world or q=hello+world.\n got: %q", got)
	}
	if strings.Contains(got, "q=hello world") {
		t.Errorf("raw space leaked into the query string.\n got: %q", got)
	}
}

// BT_A3 — SPEC A no-regression: a non-template value with a LONE brace (JSON in
// a query param) must remain percent-encoded; only the doubled {{ }} template
// delimiters are restored, so a single { stays %7B.
func TestBT_Curl_LoneBraceStaysEncoded(t *testing.T) {
	req := model.Request{
		Method: model.Method("GET"),
		URL:    "https://example.com/usage",
		Params: []model.Param{
			{Key: "filter", Value: `{"a":1}`, Enabled: true},
		},
	}
	got := ToCurl(req, model.NewCollection(""), Options{})

	// A lone '{' / '}' must NOT survive literally in the query.
	if strings.Contains(got, `filter={"a":1}`) {
		t.Errorf("lone braces left literal; want them percent-encoded.\n got: %q", got)
	}
	// And they must actually be encoded as %7B / %7D.
	if !strings.Contains(got, "%7B") {
		t.Errorf("lone '{' not encoded as %%7B.\n got: %q", got)
	}
	if !strings.Contains(got, "%7D") {
		t.Errorf("lone '}' not encoded as %%7D.\n got: %q", got)
	}
}

// BT_A4 — SPEC A: the same literal-template behaviour must also hold for the
// built URL via requestURL (the shared path used by Build), not only ToCurl.
func TestBT_RequestURL_UnresolvedTemplatesStayLiteral(t *testing.T) {
	req := model.Request{
		URL: "{{server}}/usage",
		Params: []model.Param{
			{Key: "account", Value: "{{account}}", Enabled: true},
		},
	}
	got, err := requestURL(req, Options{})
	if err != nil {
		t.Fatalf("requestURL returned error: %v", err)
	}
	if strings.Contains(got, "%7B") || strings.Contains(got, "%7b") {
		t.Errorf("built URL contains a percent-encoded brace; want none.\n got: %q", got)
	}
	if !strings.HasPrefix(got, "{{server}}/usage") {
		t.Errorf("built URL missing literal path template.\n got: %q", got)
	}
	if !strings.Contains(got, "account={{account}}") {
		t.Errorf("built URL missing literal param template.\n got: %q", got)
	}
}
