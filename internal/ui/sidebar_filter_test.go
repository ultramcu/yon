package ui

import (
	"reflect"
	"testing"

	"github.com/ultramcu/yon/internal/model"
)

// ---- requestMatchesFilter (pure helper) ----
//
// Spec: case-insensitive substring match of the (already lower-cased) query
// against any of DisplayName(), URL, Method, Name. An empty query is defined to
// match everything (the helper is total; callers gate on q != "").

func TestRequestMatchesFilter_OnName(t *testing.T) {
	req := model.Request{Name: "Login User", Method: model.MethodPost, URL: "http://x/auth"}
	if !requestMatchesFilter(req, "login") {
		t.Fatalf("expected match on Name substring")
	}
}

func TestRequestMatchesFilter_OnMethod(t *testing.T) {
	req := model.Request{Name: "Login", Method: model.MethodDelete, URL: "http://x/auth"}
	if !requestMatchesFilter(req, "delete") {
		t.Fatalf("expected match on Method")
	}
}

func TestRequestMatchesFilter_OnURL(t *testing.T) {
	req := model.Request{Name: "Login", Method: model.MethodGet, URL: "http://api.example.com/v1/widgets"}
	if !requestMatchesFilter(req, "widgets") {
		t.Fatalf("expected match on URL substring")
	}
}

func TestRequestMatchesFilter_OnDisplayName(t *testing.T) {
	// No Name set: DisplayName() derives "METHOD /path". Querying the derived path
	// must match even though no raw field equals it verbatim.
	req := model.Request{Method: model.MethodGet, URL: "http://api.example.com/users/42"}
	if dn := req.DisplayName(); dn == "" {
		t.Fatalf("precondition: DisplayName empty")
	}
	if !requestMatchesFilter(req, "/users/42") {
		t.Fatalf("expected match on derived DisplayName path; DisplayName=%q", req.DisplayName())
	}
}

func TestRequestMatchesFilter_CaseInsensitive(t *testing.T) {
	req := model.Request{Name: "GetWidgets", Method: model.MethodGet, URL: "http://x/Widgets"}
	// Caller passes a lower-cased query; the request fields contain mixed case.
	if !requestMatchesFilter(req, "getwidgets") {
		t.Fatalf("expected case-insensitive match")
	}
}

func TestRequestMatchesFilter_NoMatch(t *testing.T) {
	req := model.Request{Name: "Login", Method: model.MethodPost, URL: "http://x/auth"}
	if requestMatchesFilter(req, "zzznope") {
		t.Fatalf("expected no match")
	}
}

func TestRequestMatchesFilter_EmptyQueryMatchesAll(t *testing.T) {
	req := model.Request{Name: "Anything", Method: model.MethodGet, URL: "http://x/y"}
	if !requestMatchesFilter(req, "") {
		t.Fatalf("empty query should match everything (total helper)")
	}
}

// ---- sidebarRows() with a filter ----

func collWithFolderAndReqs(t *testing.T) (*Window, string) {
	t.Helper()
	coll := model.NewCollection("C")
	coll.Requests = []model.Request{
		{Name: "Login", Method: model.MethodPost, URL: "http://x/auth/login"},     // 0
		{Name: "ListUsers", Method: model.MethodGet, URL: "http://x/users"},       // 1
		{Name: "DeleteUser", Method: model.MethodDelete, URL: "http://x/users/1"}, // 2
	}
	w := newScopeWindow(t, coll)
	fid := w.addFolder("Auth")
	// Move "Login" (idx 0) into the Auth folder; leave 1 and 2 top-level.
	w.moveRequestToFolder(0, fid)
	w.refreshSidebar()
	return w, fid
}

func reqIdxsOf(rows []sidebarRow) []int {
	out := []int{}
	for _, r := range rows {
		if !r.IsFolder {
			out = append(out, r.ReqIdx)
		}
	}
	return out
}

func TestSidebarRows_FilterOnlyMatchingRequests(t *testing.T) {
	w, _ := collWithFolderAndReqs(t)
	w.filterQuery = "users" // matches ListUsers (1) and DeleteUser (2) by name/URL
	rows := w.sidebarRows()

	got := reqIdxsOf(rows)
	// Login (0) is in Auth folder and does NOT match "users", so excluded.
	for _, idx := range got {
		if idx == 0 {
			t.Fatalf("Login (idx 0) should be filtered out; rows=%v", got)
		}
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 matching request rows, got %d (%v)", len(got), got)
	}
}

func TestSidebarRows_FolderWithMatchShownExpandedEvenIfCollapsed(t *testing.T) {
	w, fid := collWithFolderAndReqs(t)
	// Collapse the Auth folder; without a filter its Login row would be hidden.
	w.toggleFolderCollapsed(fid)
	if f, _ := w.folderByID(fid); !f.Collapsed {
		t.Fatalf("precondition: folder should be collapsed")
	}

	w.filterQuery = "login" // matches Login (0), which lives in the collapsed folder
	rows := w.sidebarRows()

	sawHeader := false
	sawLogin := false
	for _, r := range rows {
		if r.IsFolder && r.FolderID == fid {
			sawHeader = true
		}
		if !r.IsFolder && r.ReqIdx == 0 {
			sawLogin = true
		}
	}
	if !sawHeader {
		t.Fatalf("folder header should appear when it has a match")
	}
	if !sawLogin {
		t.Fatalf("collapsed folder with a match must show EXPANDED so the match (Login) is visible")
	}
}

func TestSidebarRows_FolderWithNoMatchOmitted(t *testing.T) {
	w, fid := collWithFolderAndReqs(t)
	w.filterQuery = "users" // Auth folder holds only Login, which does NOT match
	rows := w.sidebarRows()
	for _, r := range rows {
		if r.IsFolder && r.FolderID == fid {
			t.Fatalf("folder with no matching request must be omitted")
		}
	}
}

func TestSidebarRows_TopLevelMatchesAppear(t *testing.T) {
	w, _ := collWithFolderAndReqs(t)
	w.filterQuery = "listusers"
	rows := w.sidebarRows()
	got := reqIdxsOf(rows)
	if len(got) != 1 || got[0] != 1 {
		t.Fatalf("expected only top-level ListUsers (idx 1), got %v", got)
	}
}

func TestSidebarRows_EmptyQueryEqualsUnfiltered(t *testing.T) {
	w, _ := collWithFolderAndReqs(t)

	w.filterQuery = ""
	unfiltered := w.sidebarRows()

	// Set then clear, recompute: must equal the original grouped rows exactly.
	w.filterQuery = "users"
	_ = w.sidebarRows()
	w.filterQuery = ""
	cleared := w.sidebarRows()

	if !reflect.DeepEqual(unfiltered, cleared) {
		t.Fatalf("empty-query rows differ after clearing filter:\n unfiltered=%v\n cleared=%v", unfiltered, cleared)
	}
}

// ---- Display-only guarantees ----

func TestFilter_DisplayOnly_NoDirtyNoMutation(t *testing.T) {
	w, _ := collWithFolderAndReqs(t)
	// Baseline: capture clean state and the request slice (order + contents).
	w.dirty = false
	before := append([]model.Request(nil), w.coll.Requests...)
	selBefore := w.selectedID

	w.filterQuery = "users"
	_ = w.sidebarRows()
	w.refreshSidebar()

	if w.dirty {
		t.Fatalf("filtering must NOT mark the window dirty")
	}
	if !reflect.DeepEqual(before, w.coll.Requests) {
		t.Fatalf("filtering must NOT change Collection.Requests / order")
	}
	if w.selectedID != selBefore {
		t.Fatalf("filtering must NOT change selectedID")
	}
}

func TestFilter_EntryOnChanged_NoDirty(t *testing.T) {
	w, _ := collWithFolderAndReqs(t)
	w.dirty = false
	if w.filterEntry == nil {
		t.Fatalf("filterEntry should be built by the sidebar header")
	}
	// Drive the real Entry so OnChanged runs (sets filterQuery + refreshes).
	w.filterEntry.SetText("Users")
	if w.filterQuery != "users" {
		t.Fatalf("OnChanged should store lower-cased/trimmed query, got %q", w.filterQuery)
	}
	if w.dirty {
		t.Fatalf("typing in the filter box must NOT mark dirty")
	}
}

func TestFilter_FilteredOutRequestKeepsOpenTab(t *testing.T) {
	w, _ := collWithFolderAndReqs(t)
	// Open Login (idx 0) so it has a tab, then filter it out.
	w.openRequestTab(0)
	if _, ok := w.openTabs[0]; !ok {
		t.Fatalf("precondition: Login should have an open tab")
	}

	w.filterQuery = "users" // filters Login out of the sidebar
	w.refreshSidebar()

	if _, ok := w.openTabs[0]; !ok {
		t.Fatalf("a filtered-out request with an open tab must keep its openTabs entry")
	}
}

func TestFilter_ClearRestoresFullGroupedRows(t *testing.T) {
	w, _ := collWithFolderAndReqs(t)
	full := w.sidebarRows()

	w.filterQuery = "login"
	if reflect.DeepEqual(full, w.sidebarRows()) {
		t.Fatalf("precondition: filtered rows should differ from full rows")
	}

	w.filterQuery = ""
	if !reflect.DeepEqual(full, w.sidebarRows()) {
		t.Fatalf("clearing the filter must restore the full grouped rows")
	}
}

func TestFilter_CountBadgeShowsMatchesWhenClean(t *testing.T) {
	w, _ := collWithFolderAndReqs(t)
	w.dirty = false
	w.filterQuery = "users" // 2 matches out of 3
	w.refreshSidebarCount()
	if got := w.sidebarCount.Text; got != "2" {
		t.Fatalf("clean filtered count badge = %q, want %q", got, "2")
	}

	// Clearing returns the total.
	w.filterQuery = ""
	w.refreshSidebarCount()
	if got := w.sidebarCount.Text; got != "3" {
		t.Fatalf("cleared count badge = %q, want total %q", got, "3")
	}
}
