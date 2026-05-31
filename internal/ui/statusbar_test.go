package ui

import (
	"strings"
	"testing"

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
