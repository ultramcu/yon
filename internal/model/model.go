// Package model holds Yon's pure domain types and logic: the data that
// describes a Request, a Collection, and a Response, plus a small amount of
// pure logic (auth inheritance, display-name derivation).
//
// It depends only on the standard library and imports neither Fyne nor any
// networking package (see the UI-free-core rule). The store package serializes these types
// to .yon files with encoding/json, so the JSON struct tags here define the
// on-disk schema and must stay stable.
package model

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Method is the HTTP verb of a Request. Yon ships well-known verbs as consts
// but treats Method as a free-form string, so an arbitrary custom verb the user
// types is sent verbatim.
type Method string

// Well-known HTTP methods. Method is a string type, so a Request may also carry
// any other (custom) verb not listed here.
const (
	MethodGet     Method = "GET"
	MethodPost    Method = "POST"
	MethodPut     Method = "PUT"
	MethodDelete  Method = "DELETE"
	MethodPatch   Method = "PATCH"
	MethodHead    Method = "HEAD"
	MethodOptions Method = "OPTIONS"
)

// AuthKind identifies how a Request or Collection authenticates.
//
// A Collection's Auth never uses AuthInherit; a Request may use any kind,
// where AuthInherit means "use the Collection's Auth".
type AuthKind string

// Auth kinds.
const (
	// AuthInherit means a Request defers to its Collection's Auth. It is only
	// valid on a Request, never on a Collection.
	AuthInherit AuthKind = "inherit"
	// AuthNone means no Authorization header is sent. On a Request it is an
	// explicit override that suppresses any inherited Collection Auth.
	AuthNone AuthKind = "none"
	// AuthBasic is HTTP Basic auth using Username and Password.
	AuthBasic AuthKind = "basic"
	// AuthBearer is bearer-token auth using Token.
	AuthBearer AuthKind = "bearer"
)

// Auth describes the authentication for a Request or Collection. Only the
// fields relevant to Kind are meaningful: Username/Password for AuthBasic,
// Token for AuthBearer.
type Auth struct {
	Kind     AuthKind `json:"kind"`
	Username string   `json:"username,omitempty"`
	Password string   `json:"password,omitempty"`
	Token    string   `json:"token,omitempty"`
}

// Param is a key/value pair with an Enabled flag, used for both query
// parameters and headers. A disabled Param is kept (not deleted) but is not
// applied when the Request is sent.
type Param struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Enabled bool   `json:"enabled"`
}

// BodyType identifies the kind of payload a Request carries.
type BodyType string

// Body types.
const (
	// BodyNone means the Request has no body.
	BodyNone BodyType = "none"
	// BodyJSON means the body is JSON; the sender adds an
	// "application/json" Content-Type unless one is already set.
	BodyJSON BodyType = "json"
	// BodyText means the body is raw text with no automatic Content-Type.
	BodyText BodyType = "text"
	// BodyXML means the body is XML; the sender adds an "application/xml"
	// Content-Type unless one is already set.
	BodyXML BodyType = "xml"
)

// Body is the payload of a Request. It is held on every Request regardless of
// Method and is sent as-is when Content is non-empty.
type Body struct {
	Type    BodyType `json:"type"`
	Content string   `json:"content,omitempty"`
}

// Folder is a one-level-deep grouping of Requests within a Collection. Folders
// do NOT nest (no folder ever lives inside another) and they do NOT own their
// requests: a Request points at its folder via Request.FolderID, while
// Collection.Requests stays a single flat, ordered slice. A Folder is therefore
// a display/grouping layer over the flat requests, identified by a stable ID.
type Folder struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Collapsed bool   `json:"collapsed,omitempty"`
}

// NewFolderID returns a short, collision-resistant folder id: the literal "f"
// followed by 8 hex characters (4 crypto-random bytes). Mirrors the uuid helper
// style in internal/variables. On the (vanishingly unlikely) rand failure it
// still returns a well-formed, if non-random, id.
func NewFolderID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "f00000000"
	}
	return "f" + hex.EncodeToString(b[:])
}

// Request is a single HTTP request definition the user composes and sends.
//
// FolderID is the id of the Folder this Request belongs to, or "" for a
// top-level (ungrouped) request. It is grouping metadata only: it never changes
// the Request's position in the flat Collection.Requests slice.
type Request struct {
	Name     string  `json:"name,omitempty"`
	Method   Method  `json:"method"`
	URL      string  `json:"url"`
	Params   []Param `json:"params,omitempty"`  // query parameters
	Headers  []Param `json:"headers,omitempty"` // request headers
	Auth     Auth    `json:"auth"`
	Body     Body    `json:"body"`
	FolderID string  `json:"folderId,omitempty"` // owning Folder id, or "" for top-level
}

// Collection is a flat, ordered list of Requests persisted as one .yon file.
// Version is the schema version (starting at 1) and Auth is the
// Collection-level default whose Kind is one of none/basic/bearer.
//
// Variables are collection-scoped {{template}} values (lowest precedence, below
// the active environment). ActiveEnvironment names the environment selected for
// this collection; the environments themselves live in sibling files, not in the
// .yon (see the store package), so secrets never land in the committed file.
type Collection struct {
	Version           int        `json:"version"`
	Name              string     `json:"name"`
	Auth              Auth       `json:"auth"`
	Variables         []Variable `json:"variables,omitempty"`
	ActiveEnvironment string     `json:"activeEnvironment,omitempty"`
	Folders           []Folder   `json:"folders,omitempty"`
	Requests          []Request  `json:"requests,omitempty"`
}

// Response is the result of sending a Request. It is read-only data: status,
// status text, response headers, body bytes, and timing/size metadata.
type Response struct {
	Status     int           `json:"status"`
	StatusText string        `json:"statusText"`
	Headers    []Param       `json:"headers,omitempty"`
	Body       []byte        `json:"body,omitempty"`
	Duration   time.Duration `json:"duration"`
	Size       int64         `json:"size"`
}

// NewCollection returns an empty Collection at schema version 1 with the given
// name, no Auth (Kind AuthNone) and no Requests. It is the starting point for
// the "New" action.
func NewCollection(name string) Collection {
	return Collection{
		Version:  1,
		Name:     name,
		Auth:     Auth{Kind: AuthNone},
		Requests: nil,
	}
}

// ResolveAuth returns the Auth actually applied when req is sent: the
// Collection's Auth when the Request's Auth Kind is AuthInherit, otherwise the
// Request's own Auth. An explicit Request Auth of AuthNone is returned as-is,
// suppressing any inherited Collection Auth.
func ResolveAuth(req Request, coll Collection) Auth {
	if req.Auth.Kind == AuthInherit {
		return coll.Auth
	}
	return req.Auth
}

// DisplayName returns the Request's Name, or a derived "METHOD /path" label
// when Name is empty. The path is taken from the URL; if the URL cannot be
// parsed, it falls back to the raw URL, and if that is empty too, to just the
// Method.
func (r Request) DisplayName() string {
	if r.Name != "" {
		return r.Name
	}

	method := string(r.Method)

	raw := strings.TrimSpace(r.URL)
	if raw == "" {
		return method
	}

	u, err := url.Parse(raw)
	if err != nil {
		return method + " " + raw
	}

	path := u.Path
	if u.Opaque != "" { // e.g. "mailto:foo" or schemeless "host:port" forms
		path = u.Opaque
	}
	if path == "" {
		// No path component (e.g. "https://example.com"); fall back to the
		// host, then to the raw URL, so the label is never just bare.
		if u.Host != "" {
			return method + " " + u.Host
		}
		return method + " " + raw
	}

	return fmt.Sprintf("%s %s", method, path)
}
