package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/ultramcu/yon/internal/model"
)

// TestCurlTab_PresentAndLive proves the "Copy as cURL" command now lives in a
// "cURL" sub-tab of the request editor (it used to be a top-bar button + dialog).
//
// Fail-before (conceptually): before the move there was no "cURL" segment and no
// rt.curlEntry — the segTabs ended at "Tests", so the label assertion and the
// curlEntry nil-check below would both fail.
//
// Pass-after: the segTabs include a "cURL" segment, rt.curlEntry holds the curl
// command (begins with "curl", contains the request URL), and editing the URL +
// commit re-renders it live.
func TestCurlTab_PresentAndLive(t *testing.T) {
	fyneApp := test.NewApp()
	app := New(fyneApp)

	coll := model.NewCollection("Curl")
	coll.Requests = append(coll.Requests, model.Request{
		Name:   "Ping",
		Method: model.MethodGet,
		URL:    "https://example.com/ping",
	})

	w := app.OpenCollectionWindow(coll, "/tmp/curl.yon")
	w.openRequestTab(0)

	rt, ok := w.openTabs[0]
	if !ok {
		t.Fatal("request tab 0 was not opened")
	}

	// The segTabs must include a "cURL" segment, after the "Tests" one.
	var haveCurl bool
	var labels []string
	for _, b := range rt.segTabs.segs {
		labels = append(labels, b.label)
		if b.label == "cURL" {
			haveCurl = true
		}
	}
	if !haveCurl {
		t.Fatalf("segTabs %v has no \"cURL\" segment", labels)
	}

	// The entry must have been built and seeded.
	if rt.curlEntry == nil {
		t.Fatal("rt.curlEntry is nil — cURL tab not built")
	}

	// A fresh tab must NOT be dirty: refreshCurl only SetText's the curlEntry
	// (which has no OnChanged), so seeding it must not commit.
	if rt.dirty {
		t.Error("fresh request tab is dirty after building the cURL tab")
	}

	// Initial curl: begins with "curl" and contains the request URL.
	got := rt.curlEntry.Text
	if !strings.HasPrefix(got, "curl") {
		t.Errorf("curl text = %q, want it to begin with \"curl\"", got)
	}
	if !strings.Contains(got, "https://example.com/ping") {
		t.Errorf("curl text = %q, want it to contain the request URL", got)
	}

	// Live refresh: changing the URL + commit re-renders the curl text.
	rt.urlEntry.SetText("https://example.com/changed")
	rt.commit()
	updated := rt.curlEntry.Text
	if !strings.Contains(updated, "https://example.com/changed") {
		t.Errorf("after URL edit + commit, curl text = %q, want the new URL", updated)
	}
	if strings.Contains(updated, "/ping") {
		t.Errorf("after URL edit + commit, curl text = %q, still shows the old URL", updated)
	}
}

// TestCurlTab_ReflectsPerRequestOptions closes a coverage gap: the cURL tab must
// apply the request's per-request Options (via ApplyRequestOptions) so the shown
// command carries -k (insecure TLS) and --max-time (timeout). Without this a
// regression that dropped ApplyRequestOptions in curlCommand would go unnoticed.
func TestCurlTab_ReflectsPerRequestOptions(t *testing.T) {
	fyneApp := test.NewApp()
	app := New(fyneApp)

	insecure := true
	timeout := 7
	coll := model.NewCollection("CurlOpts")
	coll.Requests = append(coll.Requests, model.Request{
		Name:   "Ping",
		Method: model.MethodGet,
		URL:    "https://example.com/ping",
		Options: &model.RequestOptions{
			InsecureTLS:    &insecure,
			TimeoutSeconds: &timeout,
		},
	})

	w := app.OpenCollectionWindow(coll, "/tmp/curlopts.yon")
	w.openRequestTab(0)
	rt := w.openTabs[0]

	got := rt.curlEntry.Text
	if !strings.Contains(got, "-k") {
		t.Errorf("curl tab missing -k for InsecureTLS override:\n%s", got)
	}
	if !strings.Contains(got, "--max-time 7") {
		t.Errorf("curl tab missing --max-time 7 for TimeoutSeconds override:\n%s", got)
	}
}
