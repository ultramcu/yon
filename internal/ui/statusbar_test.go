package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"github.com/ultramcu/yon/internal/model"
)

func newTestWindow(coll model.Collection) *Window {
	return newWindow(New(test.NewApp()), coll, "")
}

func TestStatusBar_ReflectsActiveTabResponse(t *testing.T) {
	coll := model.NewCollection("C")
	coll.Requests = []model.Request{{Name: "R", Method: model.MethodGet, URL: "http://localhost:7878/get"}}
	w := newTestWindow(coll)
	w.openRequestTab(0)

	w.openTabs[0].lastResp = &model.Response{Status: 200, StatusText: "OK", Duration: 12_000_000, Size: 248}
	w.updateStatusBar()

	if !strings.Contains(w.sbStatus.Text, "200") || !strings.Contains(w.sbStatus.Text, "OK") {
		t.Fatalf("status text = %q", w.sbStatus.Text)
	}
	if !strings.Contains(w.sbReqInfo.Text, "GET") || !strings.Contains(w.sbReqInfo.Text, "/get") {
		t.Fatalf("reqinfo text = %q", w.sbReqInfo.Text)
	}
	if !strings.Contains(w.sbMeta.Text, "248") {
		t.Fatalf("meta text = %q", w.sbMeta.Text)
	}
}

func TestStatusBar_IdleWhenNoTab(t *testing.T) {
	w := newTestWindow(model.NewCollection("C"))
	w.updateStatusBar()
	if w.sbStatus.Text != "Ready" || w.sbReqInfo.Text != "" {
		t.Fatalf("idle bar = %q / %q", w.sbStatus.Text, w.sbReqInfo.Text)
	}
}

// findVersionHBox locates the (left) container that directly holds the version
// label and returns its children, so a test can assert ordering.
func findVersionHBox(obj fyne.CanvasObject, target fyne.CanvasObject) []fyne.CanvasObject {
	if c, ok := obj.(*fyne.Container); ok {
		for _, ch := range c.Objects {
			if ch == target {
				return c.Objects
			}
		}
		for _, ch := range c.Objects {
			if got := findVersionHBox(ch, target); got != nil {
				return got
			}
		}
	}
	return nil
}

func TestStatusBar_VersionIsFirstOnLeft(t *testing.T) {
	w := newTestWindow(model.NewCollection("C"))

	if w.sbVersion == nil {
		t.Fatal("sbVersion is nil")
	}
	if w.sbStatus == nil || w.sbReqInfo == nil {
		t.Fatal("status bar fields missing")
	}

	// Text mirrors currentVersion(): "v<ver>" for a build, "dev" otherwise.
	// buildVersion is package-global and sticky, so assert against whatever
	// currentVersion() reports rather than hard-coding a value.
	want := "dev"
	if v := currentVersion(); v != "" {
		want = "v" + v
	}
	if w.sbVersion.Text != want {
		t.Fatalf("version text = %q, want %q", w.sbVersion.Text, want)
	}

	bar := w.buildStatusBar()
	children := findVersionHBox(bar, w.sbVersion)
	if children == nil {
		t.Fatal("could not find the left HBox containing sbVersion")
	}
	if children[0] != w.sbVersion {
		t.Fatalf("sbVersion is not the first child of the left HBox")
	}
	// sbVersion must come before sbStatus.
	vi, si := -1, -1
	for i, ch := range children {
		if ch == w.sbVersion {
			vi = i
		}
		if ch == w.sbStatus {
			si = i
		}
	}
	if vi < 0 || si < 0 || vi >= si {
		t.Fatalf("expected version (idx %d) before status (idx %d)", vi, si)
	}
}

func TestDirtyDot_SetOnEditClearOnSave(t *testing.T) {
	coll := model.NewCollection("C")
	coll.Requests = []model.Request{{Name: "R", Method: model.MethodGet, URL: "http://x/y"}}
	w := newTestWindow(coll)
	w.openRequestTab(0)
	rt := w.openTabs[0]

	if strings.HasPrefix(rt.tab.Text, "●") {
		t.Fatalf("fresh tab should not be dirty: %q", rt.tab.Text)
	}

	rt.nameEntry.SetText("Renamed") // OnChanged -> commit -> dirty
	if !rt.dirty {
		t.Fatal("an edit should mark the tab dirty")
	}
	if !strings.HasPrefix(rt.tab.Text, "● ") {
		t.Fatalf("dirty tab should show ●, got %q", rt.tab.Text)
	}

	w.clearTabsDirty() // what a successful Save does
	if rt.dirty || strings.HasPrefix(rt.tab.Text, "●") {
		t.Fatalf("clear failed: dirty=%v text=%q", rt.dirty, rt.tab.Text)
	}
}
