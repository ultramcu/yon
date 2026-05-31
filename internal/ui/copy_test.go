package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/ultramcu/yon/internal/model"
)

func TestCopyBody_PrettyAndRaw(t *testing.T) {
	test.NewApp()
	rv := newResponseView(test.NewWindow(nil))

	raw := []byte(`{"a":1,"b":[2,3]}`)
	rv.setResponse(model.Response{Status: 200, StatusText: "OK", Body: raw})

	// Pretty mode (default): clipboard gets pretty-printed JSON (indented).
	rv.setPretty(true)
	rv.copyBody()
	got := fyne.CurrentApp().Clipboard().Content()
	if !strings.Contains(got, "\n") || !strings.Contains(got, `"a"`) {
		t.Fatalf("pretty copy should be indented JSON, got %q", got)
	}

	// Raw mode: clipboard gets the exact raw bytes.
	rv.setPretty(false)
	rv.copyBody()
	if c := fyne.CurrentApp().Clipboard().Content(); c != string(raw) {
		t.Fatalf("raw copy = %q, want %q", c, string(raw))
	}
}

func TestCopyButton_VisibilityAndSegment(t *testing.T) {
	test.NewApp()
	rv := newResponseView(test.NewWindow(nil))

	if rv.copyBtn.Visible() {
		t.Fatal("Copy button should be hidden before any response")
	}
	rv.setResponse(model.Response{Status: 200, StatusText: "OK", Body: []byte("{}")})
	if !rv.copyBtn.Visible() {
		t.Fatal("Copy button should be shown after a response")
	}
	rv.setPending()
	if rv.copyBtn.Visible() {
		t.Fatal("Copy button should hide while pending")
	}

	// Pretty/Raw segmented control: active segment is HighImportance.
	rv.setPretty(false)
	if rv.rawBtn.Importance != widget.HighImportance || rv.prettyBtn.Importance != widget.MediumImportance {
		t.Fatal("Raw should be the active segment after setPretty(false)")
	}
	rv.setPretty(true)
	if rv.prettyBtn.Importance != widget.HighImportance || rv.rawBtn.Importance != widget.MediumImportance {
		t.Fatal("Pretty should be the active segment after setPretty(true)")
	}
}
