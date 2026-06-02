// Package importer imports Collection v2.1 JSON documents into Yon's
// own model.Collection. The source format has a richer data model than Yon, so the import
// is lossy: nested folders are flattened into "Folder / Request" names,
// unsupported HTTP methods are skipped, and unsupported auth/body kinds are
// downgraded. A Report records everything that was skipped or downgraded so the
// caller can show the user what changed.
//
// It depends only on the standard library and model, never on Fyne or any
// networking package (see the UI-free-core rule).
package importer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/ultramcu/yon/internal/model"
)

// Report summarizes the losses incurred while importing a collection.
//
// SkippedRequests lists requests that were dropped entirely (one entry per
// request, not deduped), e.g. "Folder / Login — unsupported method PATCH".
// Notes lists non-fatal downgrade messages (e.g. a form-data body dropped);
// Notes are deduped so an identical message appears at most once.
type Report struct {
	SkippedRequests []string // entries like "Folder / Login — unsupported method PATCH"
	Notes           []string // deduped non-fatal downgrade notes
}

// add appends a note unless an identical one is already present.
func (r *Report) add(note string) {
	for _, existing := range r.Notes {
		if existing == note {
			return
		}
	}
	r.Notes = append(r.Notes, note)
}

// pmCollection mirrors the top-level Collection v2.1 document. The item field is a
// pointer so we can tell "missing" (not a collection) from "present but empty".
type pmCollection struct {
	Info pmInfo    `json:"info"`
	Auth *pmAuth   `json:"auth"`
	Item *[]pmNode `json:"item"`
}

// pmInfo holds the parts of the v2.1 "info" block Yon uses.
type pmInfo struct {
	Name string `json:"name"`
}

// pmNode is one entry in an item[] array: a folder when Item is non-nil, a
// request when Request is non-nil.
type pmNode struct {
	Name    string     `json:"name"`
	Item    *[]pmNode  `json:"item"`
	Request *pmRequest `json:"request"`
}

// pmRequest is the source "request" object.
type pmRequest struct {
	Method string          `json:"method"`
	Header []pmHeader      `json:"header"`
	URL    json.RawMessage `json:"url"` // string OR object
	Auth   *pmAuth         `json:"auth"`
	Body   *pmBody         `json:"body"`
}

// pmHeader is one entry of the request "header" array.
type pmHeader struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	Disabled bool   `json:"disabled"`
}

// pmAuth is a source auth block. The kind-specific arrays (bearer/basic) hold
// key/value pairs.
type pmAuth struct {
	Type   string       `json:"type"`
	Bearer []pmAuthAttr `json:"bearer"`
	Basic  []pmAuthAttr `json:"basic"`
}

// pmAuthAttr is one key/value attribute inside an auth block.
type pmAuthAttr struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// pmURLObject is the object form of a request URL.
type pmURLObject struct {
	Raw   string         `json:"raw"`
	Query []pmQueryParam `json:"query"`
}

// pmQueryParam is one entry of a URL "query" array.
type pmQueryParam struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	Disabled bool   `json:"disabled"`
}

// pmBody is the request "body" object.
type pmBody struct {
	Mode       string         `json:"mode"`
	Raw        string         `json:"raw"`
	URLEncoded []pmQueryParam `json:"urlencoded"`
	Options    *pmBodyOptions `json:"options"`
}

// pmBodyOptions carries the language hint for raw bodies.
type pmBodyOptions struct {
	Raw *pmRawOptions `json:"raw"`
}

// pmRawOptions holds the raw-body language (e.g. "json").
type pmRawOptions struct {
	Language string `json:"language"`
}

// Import parses Collection v2.1 JSON into a Yon Collection. It returns
// the Collection, a Report of anything skipped or downgraded, and an error only
// when the input is not valid JSON or is not a recognised collection file (has no
// "item" array). On success the error is nil even if the Report is non-empty.
func Import(data []byte) (model.Collection, Report, error) {
	var doc pmCollection
	if err := json.Unmarshal(data, &doc); err != nil {
		return model.Collection{}, Report{}, fmt.Errorf("parse collection JSON: %w", err)
	}
	if doc.Item == nil {
		return model.Collection{}, Report{}, fmt.Errorf(`not a recognised collection file (no "item" array)`)
	}

	var report Report

	coll := model.Collection{
		Version: 1,
		Name:    doc.Info.Name,
	}
	if coll.Name == "" {
		coll.Name = "Imported Collection"
	}

	// Collection-level auth. Inherit is meaningless at the collection level, so
	// an absent block becomes None.
	if doc.Auth == nil {
		coll.Auth = model.Auth{Kind: model.AuthNone}
	} else {
		coll.Auth, _ = parseAuth(*doc.Auth, coll.Name, &report)
	}

	walk(*doc.Item, nil, &coll, &report)

	// {{variables}} survive as literal text; flag once if any are present.
	if bytes.Contains(data, []byte("{{")) {
		report.add("{{variables}} were kept as literal text (Yon has no environment variables).")
	}

	return coll, report, nil
}

// walk traverses nodes depth-first, preserving order, accumulating the folder
// name path in prefix. Requests are appended to coll; skips and downgrades are
// recorded in report.
func walk(nodes []pmNode, prefix []string, coll *model.Collection, report *Report) {
	for _, node := range nodes {
		name := node.Name
		if name == "" {
			name = "(unnamed)"
		}
		fullName := strings.Join(append(append([]string{}, prefix...), name), " / ")

		if node.Item != nil { // folder
			walk(*node.Item, append(append([]string{}, prefix...), name), coll, report)
			continue
		}
		if node.Request == nil { // neither folder nor request: ignore
			continue
		}

		method := strings.ToUpper(node.Request.Method)
		if !isSupportedMethod(method) {
			report.SkippedRequests = append(report.SkippedRequests,
				fmt.Sprintf("%s — unsupported method %s", fullName, method))
			continue
		}

		req := model.Request{
			Name:   fullName,
			Method: model.Method(method),
		}

		req.URL, req.Params = parseURL(node.Request.URL)
		req.Headers = parseHeaders(node.Request.Header)

		if node.Request.Auth == nil {
			req.Auth = model.Auth{Kind: model.AuthInherit}
		} else {
			req.Auth, _ = parseAuth(*node.Request.Auth, fullName, report)
		}

		req.Body = parseBody(node.Request.Body, fullName, report)

		coll.Requests = append(coll.Requests, req)
	}
}

// isSupportedMethod reports whether method is one of the four Yon verbs.
func isSupportedMethod(method string) bool {
	switch model.Method(method) {
	case model.MethodGet, model.MethodPost, model.MethodPut, model.MethodDelete:
		return true
	default:
		return false
	}
}

// parseAuth converts a source auth block into a model.Auth. The owner string
// (collection or request full name) is used in the downgrade note for an
// unsupported type. The returned bool reports whether the type was recognized
// (false for an unsupported type that was downgraded to None).
func parseAuth(a pmAuth, owner string, report *Report) (model.Auth, bool) {
	switch a.Type {
	case "noauth":
		return model.Auth{Kind: model.AuthNone}, true
	case "bearer":
		return model.Auth{Kind: model.AuthBearer, Token: authAttr(a.Bearer, "token")}, true
	case "basic":
		return model.Auth{
			Kind:     model.AuthBasic,
			Username: authAttr(a.Basic, "username"),
			Password: authAttr(a.Basic, "password"),
		}, true
	default:
		report.add(fmt.Sprintf("Auth type %q on %q is not supported — set to None.", a.Type, owner))
		return model.Auth{Kind: model.AuthNone}, false
	}
}

// authAttr returns the value of the attribute with the given key, or "".
func authAttr(attrs []pmAuthAttr, key string) string {
	for _, attr := range attrs {
		if attr.Key == key {
			return attr.Value
		}
	}
	return ""
}

// parseURL extracts the base URL (everything before the first '?') and the
// query parameters from a source url field, which is either a JSON string or
// an object with "raw" and "query". It prefers the structured url.query[]; when
// that is absent it falls back to parsing the raw query string.
func parseURL(raw json.RawMessage) (string, []model.Param) {
	if len(raw) == 0 {
		return "", nil
	}

	var rawURL string
	var query []pmQueryParam

	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		rawURL = asString
	} else {
		var obj pmURLObject
		if err := json.Unmarshal(raw, &obj); err == nil {
			rawURL = obj.Raw
			query = obj.Query
		}
	}

	base := rawURL
	queryString := ""
	if i := strings.IndexByte(rawURL, '?'); i >= 0 {
		base = rawURL[:i]
		queryString = rawURL[i+1:]
	}

	var params []model.Param
	if len(query) > 0 {
		for _, q := range query {
			params = append(params, model.Param{
				Key:     q.Key,
				Value:   q.Value,
				Enabled: !q.Disabled,
			})
		}
	} else if queryString != "" {
		params = parseQueryString(queryString)
	}

	return base, params
}

// parseQueryString splits a raw "a=1&b=2" query into Params (all enabled),
// preserving order and decoding percent-escapes where possible.
func parseQueryString(qs string) []model.Param {
	var params []model.Param
	for _, pair := range strings.Split(qs, "&") {
		if pair == "" {
			continue
		}
		key, value, _ := strings.Cut(pair, "=")
		if decoded, err := url.QueryUnescape(key); err == nil {
			key = decoded
		}
		if decoded, err := url.QueryUnescape(value); err == nil {
			value = decoded
		}
		params = append(params, model.Param{Key: key, Value: value, Enabled: true})
	}
	return params
}

// parseHeaders converts source request headers into model Params.
func parseHeaders(headers []pmHeader) []model.Param {
	var params []model.Param
	for _, h := range headers {
		params = append(params, model.Param{
			Key:     h.Key,
			Value:   h.Value,
			Enabled: !h.Disabled,
		})
	}
	return params
}

// parseBody converts a source body into a model.Body, recording a downgrade
// note for urlencoded (imported as text) and form-data/file (dropped) modes.
func parseBody(body *pmBody, owner string, report *Report) model.Body {
	if body == nil {
		return model.Body{Type: model.BodyNone}
	}

	switch body.Mode {
	case "raw":
		if body.Options != nil && body.Options.Raw != nil && body.Options.Raw.Language == "json" {
			return model.Body{Type: model.BodyJSON, Content: body.Raw}
		}
		return model.Body{Type: model.BodyText, Content: body.Raw}
	case "urlencoded":
		report.add(fmt.Sprintf("urlencoded body on %q imported as text.", owner))
		values := url.Values{}
		for _, e := range body.URLEncoded {
			if e.Disabled {
				continue
			}
			values.Add(e.Key, e.Value)
		}
		return model.Body{Type: model.BodyText, Content: values.Encode()}
	case "formdata", "file":
		report.add(fmt.Sprintf("form-data/file body on %q was dropped (Yon supports none/JSON/text).", owner))
		return model.Body{Type: model.BodyNone}
	default: // "none", "", or unknown
		return model.Body{Type: model.BodyNone}
	}
}
