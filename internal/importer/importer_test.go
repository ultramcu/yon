package importer

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ultramcu/yon/internal/model"
)

// reqByName returns the imported Request with the given (already-flattened)
// Name, or fails the test. It pins the spec's flattened-name contract.
func reqByName(t *testing.T, coll model.Collection, name string) model.Request {
	t.Helper()
	for _, r := range coll.Requests {
		if r.Name == name {
			return r
		}
	}
	var names []string
	for _, r := range coll.Requests {
		names = append(names, r.Name)
	}
	t.Fatalf("no request named %q; have %v", name, names)
	return model.Request{}
}

// reqNames returns the flattened names of every imported Request, for failure
// messages.
func reqNames(coll model.Collection) []string {
	var names []string
	for _, r := range coll.Requests {
		names = append(names, r.Name)
	}
	return names
}

func mustImport(t *testing.T, data string) (model.Collection, Report) {
	t.Helper()
	coll, rep, err := Import([]byte(data))
	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}
	return coll, rep
}

// 1. Basic mapping: info.name + one GET request maps to a v1 Collection with
// one Request whose Method/URL/Name are exactly as given.
func TestImport_BasicMapping(t *testing.T) {
	const js = `{
	  "info": {"name": "My API"},
	  "item": [
	    {
	      "name": "List users",
	      "request": {
	        "method": "GET",
	        "url": {"raw": "https://h/users"}
	      }
	    }
	  ]
	}`
	coll, _ := mustImport(t, js)

	if coll.Name != "My API" {
		t.Errorf("Name = %q, want %q", coll.Name, "My API")
	}
	if coll.Version != 1 {
		t.Errorf("Version = %d, want 1", coll.Version)
	}
	if len(coll.Requests) != 1 {
		t.Fatalf("len(Requests) = %d, want 1", len(coll.Requests))
	}
	r := coll.Requests[0]
	if r.Name != "List users" {
		t.Errorf("Request.Name = %q, want %q", r.Name, "List users")
	}
	if r.Method != model.MethodGet {
		t.Errorf("Request.Method = %q, want GET", r.Method)
	}
	if r.URL != "https://h/users" {
		t.Errorf("Request.URL = %q, want %q", r.URL, "https://h/users")
	}
}

// 2. Folder flattening: nested folders are flattened into "A / B / Name" and
// document order is preserved.
func TestImport_FolderFlattening(t *testing.T) {
	const js = `{
	  "info": {"name": "C"},
	  "item": [
	    {
	      "name": "Folder",
	      "item": [
	        {
	          "name": "Sub",
	          "item": [
	            {"name": "Login", "request": {"method": "GET", "url": {"raw": "https://h/login"}}},
	            {"name": "Logout", "request": {"method": "GET", "url": {"raw": "https://h/logout"}}}
	          ]
	        }
	      ]
	    },
	    {"name": "Top", "request": {"method": "GET", "url": {"raw": "https://h/top"}}}
	  ]
	}`
	coll, _ := mustImport(t, js)

	wantOrder := []string{"Folder / Sub / Login", "Folder / Sub / Logout", "Top"}
	if len(coll.Requests) != len(wantOrder) {
		t.Fatalf("len(Requests) = %d, want %d", len(coll.Requests), len(wantOrder))
	}
	for i, want := range wantOrder {
		if coll.Requests[i].Name != want {
			t.Errorf("Requests[%d].Name = %q, want %q", i, coll.Requests[i].Name, want)
		}
	}
}

// 3. Query params: query string is stripped from the URL and modelled as
// Params (disabled flag preserved).
func TestImport_QueryParamsStrippedToParams(t *testing.T) {
	const js = `{
	  "info": {"name": "C"},
	  "item": [
	    {
	      "name": "Q",
	      "request": {
	        "method": "GET",
	        "url": {
	          "raw": "https://h/users?page=1&q=x",
	          "query": [
	            {"key": "page", "value": "1"},
	            {"key": "q", "value": "x", "disabled": true}
	          ]
	        }
	      }
	    }
	  ]
	}`
	coll, _ := mustImport(t, js)
	r := reqByName(t, coll, "Q")

	if r.URL != "https://h/users" {
		t.Errorf("URL = %q, want %q (query must be stripped)", r.URL, "https://h/users")
	}
	if strings.Contains(r.URL, "?") {
		t.Errorf("URL %q still contains a query string", r.URL)
	}
	want := []model.Param{
		{Key: "page", Value: "1", Enabled: true},
		{Key: "q", Value: "x", Enabled: false},
	}
	if !reflect.DeepEqual(r.Params, want) {
		t.Errorf("Params = %#v, want %#v", r.Params, want)
	}
}

// 4. Headers: disabled flag inverts to Enabled, in order.
func TestImport_Headers(t *testing.T) {
	const js = `{
	  "info": {"name": "C"},
	  "item": [
	    {
	      "name": "H",
	      "request": {
	        "method": "GET",
	        "url": {"raw": "https://h/x"},
	        "header": [
	          {"key": "Accept", "value": "application/json", "disabled": false},
	          {"key": "X", "value": "y", "disabled": true}
	        ]
	      }
	    }
	  ]
	}`
	coll, _ := mustImport(t, js)
	r := reqByName(t, coll, "H")

	want := []model.Param{
		{Key: "Accept", Value: "application/json", Enabled: true},
		{Key: "X", Value: "y", Enabled: false},
	}
	if !reflect.DeepEqual(r.Headers, want) {
		t.Errorf("Headers = %#v, want %#v", r.Headers, want)
	}
}

// 5. Auth mapping across the five cases plus collection-level auth.
func TestImport_AuthMapping(t *testing.T) {
	const js = `{
	  "info": {"name": "C"},
	  "auth": {"type": "bearer", "bearer": [{"key": "token", "value": "COLL"}]},
	  "item": [
	    {
	      "name": "Bear",
	      "request": {
	        "method": "GET", "url": {"raw": "https://h/a"},
	        "auth": {"type": "bearer", "bearer": [{"key": "token", "value": "T"}]}
	      }
	    },
	    {
	      "name": "Bas",
	      "request": {
	        "method": "GET", "url": {"raw": "https://h/b"},
	        "auth": {"type": "basic", "basic": [
	          {"key": "username", "value": "u"},
	          {"key": "password", "value": "p"}
	        ]}
	      }
	    },
	    {
	      "name": "No",
	      "request": {
	        "method": "GET", "url": {"raw": "https://h/c"},
	        "auth": {"type": "noauth"}
	      }
	    },
	    {
	      "name": "Inh",
	      "request": {"method": "GET", "url": {"raw": "https://h/d"}}
	    }
	  ]
	}`
	coll, _ := mustImport(t, js)

	// (e) collection-level bearer
	if coll.Auth.Kind != model.AuthBearer {
		t.Errorf("Collection.Auth.Kind = %q, want bearer", coll.Auth.Kind)
	}
	if coll.Auth.Token != "COLL" {
		t.Errorf("Collection.Auth.Token = %q, want COLL", coll.Auth.Token)
	}

	// (a) request bearer
	if got := reqByName(t, coll, "Bear").Auth; got.Kind != model.AuthBearer || got.Token != "T" {
		t.Errorf("Bear.Auth = %#v, want {bearer, token=T}", got)
	}
	// (b) request basic
	if got := reqByName(t, coll, "Bas").Auth; got.Kind != model.AuthBasic || got.Username != "u" || got.Password != "p" {
		t.Errorf("Bas.Auth = %#v, want {basic, u/p}", got)
	}
	// (c) noauth -> none
	if got := reqByName(t, coll, "No").Auth.Kind; got != model.AuthNone {
		t.Errorf("No.Auth.Kind = %q, want none", got)
	}
	// (d) no auth key -> inherit
	if got := reqByName(t, coll, "Inh").Auth.Kind; got != model.AuthInherit {
		t.Errorf("Inh.Auth.Kind = %q, want inherit", got)
	}
}

// 6. Body modes.
func TestImport_Body(t *testing.T) {
	t.Run("raw json", func(t *testing.T) {
		const js = `{
		  "info": {"name": "C"},
		  "item": [{
		    "name": "RJ",
		    "request": {
		      "method": "GET", "url": {"raw": "https://h/x"},
		      "body": {"mode": "raw", "raw": "{\"a\":1}", "options": {"raw": {"language": "json"}}}
		    }
		  }]
		}`
		coll, _ := mustImport(t, js)
		r := reqByName(t, coll, "RJ")
		if r.Body.Type != model.BodyJSON {
			t.Errorf("Body.Type = %q, want json", r.Body.Type)
		}
		if r.Body.Content != `{"a":1}` {
			t.Errorf("Body.Content = %q, want %q", r.Body.Content, `{"a":1}`)
		}
	})

	t.Run("raw xml", func(t *testing.T) {
		const js = `{
		  "info": {"name": "C"},
		  "item": [{
		    "name": "RX",
		    "request": {
		      "method": "GET", "url": {"raw": "https://h/x"},
		      "body": {"mode": "raw", "raw": "<a/>", "options": {"raw": {"language": "xml"}}}
		    }
		  }]
		}`
		coll, _ := mustImport(t, js)
		r := reqByName(t, coll, "RX")
		if r.Body.Type != model.BodyXML {
			t.Errorf("Body.Type = %q, want xml", r.Body.Type)
		}
		if r.Body.Content != "<a/>" {
			t.Errorf("Body.Content = %q, want %q", r.Body.Content, "<a/>")
		}
	})

	t.Run("raw no language", func(t *testing.T) {
		const js = `{
		  "info": {"name": "C"},
		  "item": [{
		    "name": "RT",
		    "request": {
		      "method": "GET", "url": {"raw": "https://h/x"},
		      "body": {"mode": "raw", "raw": "hello"}
		    }
		  }]
		}`
		coll, _ := mustImport(t, js)
		r := reqByName(t, coll, "RT")
		if r.Body.Type != model.BodyText {
			t.Errorf("Body.Type = %q, want text", r.Body.Type)
		}
		if r.Body.Content != "hello" {
			t.Errorf("Body.Content = %q, want %q", r.Body.Content, "hello")
		}
	})

	t.Run("urlencoded -> text with encoded pairs", func(t *testing.T) {
		const js = `{
		  "info": {"name": "C"},
		  "item": [{
		    "name": "UE",
		    "request": {
		      "method": "GET", "url": {"raw": "https://h/x"},
		      "body": {"mode": "urlencoded", "urlencoded": [
		        {"key": "a", "value": "1"},
		        {"key": "b", "value": "2"}
		      ]}
		    }
		  }]
		}`
		coll, _ := mustImport(t, js)
		r := reqByName(t, coll, "UE")
		if r.Body.Type != model.BodyText {
			t.Errorf("Body.Type = %q, want text", r.Body.Type)
		}
		if !strings.Contains(r.Body.Content, "a=1") || !strings.Contains(r.Body.Content, "b=2") {
			t.Errorf("Body.Content = %q, want it to contain a=1 and b=2", r.Body.Content)
		}
	})

	t.Run("formdata -> none + note", func(t *testing.T) {
		const js = `{
		  "info": {"name": "C"},
		  "item": [{
		    "name": "FD",
		    "request": {
		      "method": "GET", "url": {"raw": "https://h/x"},
		      "body": {"mode": "formdata", "formdata": [{"key": "f", "value": "v"}]}
		    }
		  }]
		}`
		coll, rep := mustImport(t, js)
		r := reqByName(t, coll, "FD")
		if r.Body.Type != model.BodyNone {
			t.Errorf("Body.Type = %q, want none", r.Body.Type)
		}
		found := false
		for _, n := range rep.Notes {
			if strings.Contains(n, "FD") {
				found = true
			}
		}
		if !found {
			t.Errorf("expected a Note mentioning the request name %q; Notes = %v", "FD", rep.Notes)
		}
	})

	t.Run("no body -> none", func(t *testing.T) {
		const js = `{
		  "info": {"name": "C"},
		  "item": [{
		    "name": "NB",
		    "request": {"method": "GET", "url": {"raw": "https://h/x"}}
		  }]
		}`
		coll, _ := mustImport(t, js)
		r := reqByName(t, coll, "NB")
		if r.Body.Type != model.BodyNone {
			t.Errorf("Body.Type = %q, want none", r.Body.Type)
		}
	})
}

// 7. Arbitrary methods are imported verbatim (uppercased), NOT skipped. Yon now
// supports any HTTP verb, so a PATCH/HEAD/OPTIONS request must appear in the
// imported Collection with its method intact and must NOT be reported as a
// skipped "unsupported method". This pins the new spec and FAILS on the old
// behaviour (where PATCH was dropped into Report.SkippedRequests).
func TestImport_ArbitraryMethodsImported(t *testing.T) {
	const js = `{
	  "info": {"name": "C"},
	  "item": [
	    {
	      "name": "Folder",
	      "item": [
	        {"name": "Patchy", "request": {"method": "PATCH", "url": {"raw": "https://h/p"}}}
	      ]
	    },
	    {"name": "Heady", "request": {"method": "HEAD", "url": {"raw": "https://h/he"}}},
	    {"name": "Opty", "request": {"method": "OPTIONS", "url": {"raw": "https://h/op"}}},
	    {"name": "Okay", "request": {"method": "GET", "url": {"raw": "https://h/o"}}}
	  ]
	}`
	coll, rep := mustImport(t, js)

	// Nothing is skipped any more.
	if len(rep.SkippedRequests) != 0 {
		t.Errorf("SkippedRequests = %v, want empty (no method is unsupported)", rep.SkippedRequests)
	}

	// All four requests are present, in document order, methods intact.
	wantOrder := []string{"Folder / Patchy", "Heady", "Opty", "Okay"}
	if len(coll.Requests) != len(wantOrder) {
		t.Fatalf("len(Requests) = %d, want %d (%v)", len(coll.Requests), len(wantOrder), reqNames(coll))
	}
	for i, want := range wantOrder {
		if coll.Requests[i].Name != want {
			t.Errorf("Requests[%d].Name = %q, want %q", i, coll.Requests[i].Name, want)
		}
	}

	// The PATCH request is present with Method == "PATCH".
	if got := reqByName(t, coll, "Folder / Patchy").Method; got != model.MethodPatch {
		t.Errorf("Patchy.Method = %q, want %q (PATCH must no longer be skipped)", got, model.MethodPatch)
	}
	if got := reqByName(t, coll, "Heady").Method; got != model.MethodHead {
		t.Errorf("Heady.Method = %q, want %q", got, model.MethodHead)
	}
	if got := reqByName(t, coll, "Opty").Method; got != model.MethodOptions {
		t.Errorf("Opty.Method = %q, want %q", got, model.MethodOptions)
	}
}

// 7b. A lower-case method in the source JSON is imported uppercased.
func TestImport_MethodUppercased(t *testing.T) {
	const js = `{
	  "info": {"name": "C"},
	  "item": [
	    {"name": "L", "request": {"method": "patch", "url": {"raw": "https://h/p"}}}
	  ]
	}`
	coll, rep := mustImport(t, js)

	if len(rep.SkippedRequests) != 0 {
		t.Errorf("SkippedRequests = %v, want empty", rep.SkippedRequests)
	}
	if got := reqByName(t, coll, "L").Method; got != model.MethodPatch {
		t.Errorf("Method = %q, want %q (must be uppercased)", got, model.MethodPatch)
	}
}

// 8. Variables note: a {{var}} reference yields exactly one (deduped) Note
// mentioning variables, even when it appears multiple times.
func TestImport_VariablesNoteDeduped(t *testing.T) {
	const js = `{
	  "info": {"name": "C"},
	  "item": [
	    {"name": "A", "request": {"method": "GET", "url": {"raw": "{{base_url}}/a"}}},
	    {"name": "B", "request": {"method": "GET", "url": {"raw": "{{base_url}}/b"}}}
	  ]
	}`
	_, rep := mustImport(t, js)

	count := 0
	for _, n := range rep.Notes {
		if strings.Contains(strings.ToLower(n), "variable") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("variable-mentioning Notes count = %d, want exactly 1; Notes = %v", count, rep.Notes)
	}
}

// 9. Errors: invalid JSON and a well-formed object without an "item" array both
// return a non-nil error.
func TestImport_Errors(t *testing.T) {
	if _, _, err := Import([]byte("not json")); err == nil {
		t.Errorf("Import(invalid json) error = nil, want non-nil")
	}
	if _, _, err := Import([]byte(`{"info": {"name": "C"}}`)); err == nil {
		t.Errorf("Import(no item array) error = nil, want non-nil")
	}
}
