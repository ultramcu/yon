package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/ultramcu/yon/internal/model"
)

// Blind tests for issue #28: "Copy as cURL" moves from a top-bar button + dialog
// into a TAB in the request editor's segTabs group. These tests are written from
// the contract, BLIND to the implementation:
//   - the request editor's segTabs gains a "cURL" tab, appended LAST (after Tests)
//   - requestTab gains a field curlEntry *widget.Entry holding the equivalent curl
//     command for the current request, refreshed live on each commit()
//
// They use only the contract symbols: rt.segTabs, rt.curlEntry, rt.urlEntry,
// rt.commit(). The existing segLabels(*segTabs) helper (responsetabs_test.go) is
// reused to read the sub-tab captions.

// openCurlTab builds a window from a single-request collection and returns the
// opened *requestTab for that request.
func openCurlTab(t *testing.T, req model.Request) *requestTab {
	t.Helper()
	fyneApp := test.NewApp()
	app := New(fyneApp)

	coll := model.NewCollection("CurlTab")
	coll.Requests = append(coll.Requests, req)

	w := app.OpenCollectionWindow(coll, "/tmp/curltab.yon")
	w.openRequestTab(0)
	rt, ok := w.openTabs[0]
	if !ok {
		t.Fatal("request tab 0 was not opened")
	}
	return rt
}

// TestCurlTabPresent: the request editor's segTabs include a "cURL" tab, and it
// is the LAST tab (appended after Tests).
func TestCurlTabPresent(t *testing.T) {
	rt := openCurlTab(t, model.Request{
		Name:   "Ping",
		Method: model.MethodGet,
		URL:    "https://api.example.com/ping",
	})

	if rt.segTabs == nil {
		t.Fatal("request editor should build a segTabs")
	}
	labels := segLabels(rt.segTabs)

	found := false
	for _, l := range labels {
		if l == "cURL" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("segTabs labels %v should include %q", labels, "cURL")
	}
	if len(labels) == 0 || labels[len(labels)-1] != "cURL" {
		t.Errorf("cURL must be the LAST tab; labels = %v", labels)
	}
}

// TestCurlTabReflectsRequest: for a POST to a known URL, rt.curlEntry is non-nil
// and its text is a curl command for that request.
func TestCurlTabReflectsRequest(t *testing.T) {
	const url = "https://api.example.com/widgets"
	rt := openCurlTab(t, model.Request{
		Name:   "Make widget",
		Method: model.MethodPost,
		URL:    url,
	})

	if rt.curlEntry == nil {
		t.Fatal("rt.curlEntry should be non-nil")
	}
	got := rt.curlEntry.Text
	if !strings.HasPrefix(got, "curl") {
		t.Errorf("curlEntry text should start with %q; got %q", "curl", got)
	}
	if !strings.Contains(got, url) {
		t.Errorf("curlEntry text should contain the URL %q; got %q", url, got)
	}
	if !strings.Contains(got, "-X POST") {
		t.Errorf("curlEntry text should contain %q for a POST; got %q", "-X POST", got)
	}
}

// TestCurlTabUpdatesOnEdit: changing the URL and committing refreshes the cURL
// tab live — the new URL appears and the old one is gone.
func TestCurlTabUpdatesOnEdit(t *testing.T) {
	const oldURL = "https://api.example.com/widgets"
	const newURL = "https://changed.example.com/x"
	rt := openCurlTab(t, model.Request{
		Name:   "Make widget",
		Method: model.MethodPost,
		URL:    oldURL,
	})

	if rt.curlEntry == nil {
		t.Fatal("rt.curlEntry should be non-nil")
	}
	if !strings.Contains(rt.curlEntry.Text, oldURL) {
		t.Fatalf("setup: curlEntry should contain old URL %q; got %q", oldURL, rt.curlEntry.Text)
	}

	rt.urlEntry.SetText(newURL)
	rt.commit()

	got := rt.curlEntry.Text
	if !strings.Contains(got, newURL) {
		t.Errorf("after edit+commit, curlEntry should contain new URL %q; got %q", newURL, got)
	}
	if strings.Contains(got, oldURL) {
		t.Errorf("after edit+commit, curlEntry should NOT contain old URL %q; got %q", oldURL, got)
	}
}

// TestFreshTabNotDirtyWithCurl: opening a fresh request leaves rt.dirty false —
// building/refreshing the cURL tab must not dirty the tab.
func TestFreshTabNotDirtyWithCurl(t *testing.T) {
	rt := openCurlTab(t, model.Request{
		Name:   "Ping",
		Method: model.MethodGet,
		URL:    "https://api.example.com/ping",
	})
	if rt.dirty {
		t.Error("a freshly opened request tab must not be dirty after cURL tab construction")
	}
}
