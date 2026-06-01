package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/ultramcu/yon/internal/model"
)

func TestResponseActions_ShownOnResponseAndPopout(t *testing.T) {
	test.NewApp()
	rv := newResponseView(test.NewWindow(nil))
	btns := []*widget.Button{rv.copyBtn, rv.saveBtn, rv.popoutBtn}

	for _, b := range btns {
		if b.Visible() {
			t.Fatal("copy/save/popout should be hidden before any response")
		}
	}
	rv.setResponse(model.Response{Status: 200, StatusText: "OK", Body: []byte(`{"ok":true}`)})
	for _, b := range btns {
		if !b.Visible() {
			t.Fatal("copy/save/popout should show once there is a response")
		}
	}

	rv.showPopout() // must not panic (opens a separate window)

	rv.setPending()
	for _, b := range btns {
		if b.Visible() {
			t.Fatal("copy/save/popout should hide while pending")
		}
	}
}
