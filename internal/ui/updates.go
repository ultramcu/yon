package ui

import (
	"context"
	"fmt"
	"image/color"
	"net/url"
	"runtime"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/ultramcu/yon/internal/updater"
)

// buildVersion is the running build's version. main sets it from the embedded
// FyneApp.toml via SetBuildVersion; it can also be injected at link time
// (-ldflags "-X 'github.com/ultramcu/yon/internal/ui.buildVersion=0.3.0'"),
// which takes precedence. It is preferred over the Fyne metadata, which only
// populates under `fyne package` (and is "0.0.1" for a plain go build).
var buildVersion string

// SetBuildVersion records the running version when it was not already injected
// at link time. main calls it with the version parsed from the embedded
// FyneApp.toml, so every build — go build, fyne package, every platform — knows
// its own version. An ldflags-set buildVersion wins (a custom -X override is
// preserved).
func SetBuildVersion(v string) {
	if buildVersion == "" {
		buildVersion = v
	}
}

// currentVersion is the running build's version: the ldflags override if set,
// otherwise the FyneApp.toml version embedded by `fyne package`. Empty for plain
// `go run` / `go build` dev builds, so the update check stays quiet on them.
func currentVersion() string {
	if buildVersion != "" {
		return buildVersion
	}
	if app := fyne.CurrentApp(); app != nil {
		v := app.Metadata().Version
		// Fyne reports "0.0.1" for un-packaged (`go run` / `go build`) builds;
		// treat that placeholder as "no version" so dev builds don't claim to be
		// out of date against the latest release.
		if v != "" && v != "0.0.1" {
			return v
		}
	}
	return ""
}

// buildUpdateBanner creates the (initially hidden) "update available" notice that
// sits across the top of the window.
func (w *Window) buildUpdateBanner() fyne.CanvasObject {
	w.updateLabel = widget.NewLabel("")

	download := widget.NewButtonWithIcon("Download", theme.DownloadIcon(), w.downloadUpdate)
	download.Importance = widget.HighImportance
	view := widget.NewButton("View Release", func() {
		if u, err := url.Parse(w.pendingRel.HTMLURL); err == nil {
			w.app.fyneApp.OpenURL(u)
		}
	})
	dismiss := widget.NewButtonWithIcon("", theme.CancelIcon(), func() { w.updateBanner.Hide() })
	dismiss.Importance = widget.LowImportance

	bg := canvas.NewRectangle(color.NRGBA{R: 0x15, G: 0x65, B: 0xc0, A: 0x22}) // subtle blue
	row := container.NewBorder(nil, nil, w.updateLabel, container.NewHBox(download, view, dismiss))
	w.updateBanner = container.NewStack(bg, container.NewPadded(row))
	w.updateBanner.Hide()
	return w.updateBanner
}

// checkForUpdates queries GitHub for the latest release in the background. When a
// newer version is found it reveals the banner. When manual is true it also
// reports "up to date" / errors via a dialog; an automatic (startup) check stays
// silent unless there is something to download.
func (w *Window) checkForUpdates(manual bool) {
	cur := currentVersion()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), updater.CheckTimeout)
		defer cancel()
		rel, err := updater.Latest(ctx)

		fyne.Do(func() {
			if err != nil {
				if manual {
					dialog.ShowError(fmt.Errorf("couldn't check for updates: %w", err), w.win)
				}
				return
			}
			w.applyUpdateResult(cur, rel, manual)
		})
	}()
}

// applyUpdateResult is the synchronous decision for a fetched release: reveal the
// banner when a downloadable newer version exists, otherwise (manual only) report
// the status via a dialog. Split out from checkForUpdates so it is unit-testable.
func (w *Window) applyUpdateResult(cur string, rel updater.Release, manual bool) {
	if updater.IsNewer(cur, rel.Version) {
		if asset, ok := rel.AssetFor(runtime.GOOS); ok {
			w.pendingRel, w.pendingAsset = rel, asset
			w.updateLabel.SetText(fmt.Sprintf("Yon %s is available — you have %s.", rel.TagName, cur))
			w.updateBanner.Show()
		} else if manual {
			dialog.ShowInformation("Update available",
				fmt.Sprintf("Yon %s is available. Open the releases page to download it.", rel.TagName), w.win)
		}
		return
	}

	if manual {
		if cur == "" {
			dialog.ShowInformation("Development build",
				fmt.Sprintf("The latest release is %s.\nYou're running a development build.", rel.TagName), w.win)
		} else {
			dialog.ShowInformation("You're up to date",
				fmt.Sprintf("Yon %s is the latest version.", cur), w.win)
		}
	}
}

// downloadUpdate fetches the pending release's asset into the Downloads folder,
// reveals it in the file manager, and tells the user where it landed. It does
// not install anything.
func (w *Window) downloadUpdate() {
	rel, asset := w.pendingRel, w.pendingAsset
	if asset.URL == "" {
		return
	}
	w.updateBanner.Hide()

	progress := dialog.NewCustomWithoutButtons("Downloading "+asset.Name, widget.NewProgressBarInfinite(), w.win)
	progress.Show()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), updater.DownloadTimeout)
		defer cancel()
		path, err := updater.Download(ctx, asset, updater.DownloadsDir())

		fyne.Do(func() {
			progress.Hide()
			if err != nil {
				dialog.ShowError(fmt.Errorf("download failed: %w", err), w.win)
				return
			}
			_ = updater.Reveal(path)
			dialog.ShowInformation("Download complete",
				fmt.Sprintf("Saved to:\n%s\n\nOpen it to install Yon %s.", path, rel.TagName), w.win)
		})
	}()
}
