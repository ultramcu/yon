package yonner

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/ultramcu/yon/internal/model"
)

// requestURL parses req.URL and appends its enabled Query Parameters, preserving
// the existing query and order (shared by Build and ToCurl).
func requestURL(req model.Request) (string, error) {
	u, err := url.Parse(strings.TrimSpace(req.URL))
	if err != nil {
		return "", err
	}
	var qb strings.Builder
	qb.WriteString(u.RawQuery)
	for _, p := range req.Params {
		if !p.Enabled {
			continue
		}
		if qb.Len() > 0 {
			qb.WriteByte('&')
		}
		qb.WriteString(url.QueryEscape(p.Key))
		qb.WriteByte('=')
		qb.WriteString(url.QueryEscape(p.Value))
	}
	u.RawQuery = qb.String()
	return u.String(), nil
}

// ToCurl renders req as an equivalent `curl` command line, matching how Send
// builds and sends it: query params, enabled headers, resolved auth (explicit
// Authorization header wins), JSON auto Content-Type, and the body — plus the
// connection Options (-L follow redirects, -k insecure TLS, --max-time).
func ToCurl(req model.Request, coll model.Collection, opts Options) string {
	var b strings.Builder
	b.WriteString("curl")
	if opts.FollowRedirects {
		b.WriteString(" -L")
	}
	if opts.InsecureTLS {
		b.WriteString(" -k")
	}
	if secs := int(opts.Timeout.Seconds()); secs > 0 {
		fmt.Fprintf(&b, " --max-time %d", secs)
	}

	method := string(req.Method)
	if method == "" {
		method = http.MethodGet
	}
	if method != http.MethodGet {
		b.WriteString(" -X " + method)
	}

	u := strings.TrimSpace(req.URL)
	if full, err := requestURL(req); err == nil {
		u = full
	}
	b.WriteString(" " + shellQuote(u))

	userSetAuth, userSetCT := false, false
	for _, h := range req.Headers {
		if !h.Enabled {
			continue
		}
		if strings.EqualFold(h.Key, headerAuthorization) {
			userSetAuth = true
		}
		if strings.EqualFold(h.Key, headerContentType) {
			userSetCT = true
		}
		fmt.Fprintf(&b, " -H %s", shellQuote(h.Key+": "+h.Value))
	}

	if !userSetAuth {
		switch auth := model.ResolveAuth(req, coll); auth.Kind {
		case model.AuthBasic:
			fmt.Fprintf(&b, " -u %s", shellQuote(auth.Username+":"+auth.Password))
		case model.AuthBearer:
			fmt.Fprintf(&b, " -H %s", shellQuote("Authorization: Bearer "+auth.Token))
		}
	}

	if req.Body.Type != model.BodyNone && req.Body.Content != "" {
		if req.Body.Type == model.BodyJSON && !userSetCT {
			fmt.Fprintf(&b, " -H %s", shellQuote("Content-Type: application/json"))
		}
		fmt.Fprintf(&b, " --data-raw %s", shellQuote(req.Body.Content))
	}

	return b.String()
}

// shellQuote wraps s in single quotes, escaping embedded single quotes, so the
// value is safe to paste into a POSIX shell.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
