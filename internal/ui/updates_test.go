package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/ultramcu/yon/internal/model"
	"github.com/ultramcu/yon/internal/updater"
)

func newUpdateTestWindow(t *testing.T) *Window {
	t.Helper()
	a := test.NewApp()
	app := New(a)
	return newWindow(app, model.NewCollection("T"), "")
}

// relWithAllAssets returns a release carrying every platform asset, so AssetFor
// resolves regardless of the host GOOS the test runs on (linux + macOS in CI).
func relWithAllAssets(version string) updater.Release {
	return updater.Release{
		TagName: "v" + version,
		Version: version,
		HTMLURL: "https://github.com/ultramcu/yon/releases/tag/v" + version,
		Assets: []updater.Asset{
			{Name: "Yon-v" + version + "-macos-universal.zip", URL: "https://example.com/mac"},
			{Name: "Yon-v" + version + "-windows.zip", URL: "https://example.com/win"},
			{Name: "Yon-v" + version + "-linux.tar.xz", URL: "https://example.com/linux"},
		},
	}
}

func TestApplyUpdateResult_NewerShowsBanner(t *testing.T) {
	w := newUpdateTestWindow(t)
	if w.updateBanner.Visible() {
		t.Fatal("banner should start hidden")
	}
	w.applyUpdateResult("0.3.0", relWithAllAssets("0.4.0"), false)
	if !w.updateBanner.Visible() {
		t.Fatal("banner should show when a newer release is available")
	}
	if w.pendingAsset.URL == "" {
		t.Fatal("a platform asset should be selected for download")
	}
}

func TestApplyUpdateResult_UpToDateStaysHidden(t *testing.T) {
	w := newUpdateTestWindow(t)
	w.applyUpdateResult("0.4.0", relWithAllAssets("0.4.0"), false)
	if w.updateBanner.Visible() {
		t.Fatal("banner should stay hidden when already up to date")
	}
}

func TestSettings_CheckUpdatesPersist(t *testing.T) {
	a := test.NewApp()
	p := a.Preferences()
	if loadSettings(p).CheckUpdatesOnLaunch {
		t.Fatal("check-updates default should be off (offline/no-telemetry promise)")
	}
	s := loadSettings(p)
	s.CheckUpdatesOnLaunch = true
	s.save(p)
	if !loadSettings(p).CheckUpdatesOnLaunch {
		t.Fatal("check-updates preference should persist")
	}
}
