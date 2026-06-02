package ui

import (
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/ultramcu/yon/internal/yonner"
)

// Preference keys for the app-level connection Settings.
const (
	prefKeyTimeoutSeconds = "settings.timeoutSeconds"
	prefKeyInsecureTLS    = "settings.insecureTLS"
	prefKeyThemeID        = "settings.themeID"
	prefKeyCheckUpdates   = "settings.checkUpdatesOnLaunch"
)

// defaultTimeout is used when no timeout preference has been stored yet.
const defaultTimeout = 30 * time.Second

// Settings holds the app-level connection defaults: a request Timeout and an
// "Allow insecure TLS" toggle. These are global (not per-Request in v1) and are
// persisted via Preferences.
type Settings struct {
	Timeout     time.Duration
	InsecureTLS bool
	ThemeID     string
	// CheckUpdatesOnLaunch opts in to a one-shot background update check on
	// startup. Off by default to honour Yon's offline / no-telemetry promise; a
	// manual "Check for Updates…" is always available regardless of this flag.
	CheckUpdatesOnLaunch bool
}

// loadSettings reads the Settings from Preferences, falling back to the v1
// defaults (30s timeout, secure TLS).
func loadSettings(p fyne.Preferences) Settings {
	secs := p.IntWithFallback(prefKeyTimeoutSeconds, int(defaultTimeout/time.Second))
	if secs <= 0 {
		secs = int(defaultTimeout / time.Second)
	}
	return Settings{
		Timeout:              time.Duration(secs) * time.Second,
		InsecureTLS:          p.BoolWithFallback(prefKeyInsecureTLS, false),
		ThemeID:              p.StringWithFallback(prefKeyThemeID, defaultThemeID),
		CheckUpdatesOnLaunch: p.BoolWithFallback(prefKeyCheckUpdates, false),
	}
}

// save persists the Settings to Preferences.
func (s Settings) save(p fyne.Preferences) {
	p.SetInt(prefKeyTimeoutSeconds, int(s.Timeout/time.Second))
	p.SetBool(prefKeyInsecureTLS, s.InsecureTLS)
	p.SetString(prefKeyThemeID, s.ThemeID)
	p.SetBool(prefKeyCheckUpdates, s.CheckUpdatesOnLaunch)
}

// options converts the Settings into yonner.Options for a send. FollowRedirects
// is always true in v1 (no per-request override).
func (s Settings) options() yonner.Options {
	return yonner.Options{
		Timeout:         s.Timeout,
		InsecureTLS:     s.InsecureTLS,
		FollowRedirects: true,
	}
}

// showSettingsDialog presents the app Settings editor (timeout + insecure TLS),
// persisting the result to Preferences and updating the in-memory Settings so
// the next send uses the new values.
func (a *App) showSettingsDialog(parent fyne.Window) {
	timeoutEntry := widget.NewEntry()
	timeoutEntry.SetText(strconv.Itoa(int(a.settings.Timeout / time.Second)))
	timeoutEntry.Validator = func(s string) error {
		n, err := strconv.Atoi(s)
		if err != nil || n <= 0 {
			return errInvalidTimeout
		}
		return nil
	}

	insecureCheck := widget.NewCheck("", nil)
	insecureCheck.SetChecked(a.settings.InsecureTLS)

	updatesCheck := widget.NewCheck("Check GitHub for a newer release on startup", nil)
	updatesCheck.SetChecked(a.settings.CheckUpdatesOnLaunch)

	themeSelect := widget.NewSelect(themeNames(), nil)
	themeSelect.SetSelected(themeNameByID(a.settings.ThemeID))

	items := []*widget.FormItem{
		widget.NewFormItem("Appearance", themeSelect),
		widget.NewFormItem("Timeout (seconds)", timeoutEntry),
		widget.NewFormItem("Allow insecure TLS", insecureCheck),
		widget.NewFormItem("Updates", updatesCheck),
	}

	dialog.ShowForm("Settings", "Save", "Cancel", items, func(ok bool) {
		if !ok {
			return
		}
		secs, err := strconv.Atoi(timeoutEntry.Text)
		if err != nil || secs <= 0 {
			secs = int(defaultTimeout / time.Second)
		}
		a.settings.Timeout = time.Duration(secs) * time.Second
		a.settings.InsecureTLS = insecureCheck.Checked
		a.settings.ThemeID = themeIDByName(themeSelect.Selected)
		a.settings.CheckUpdatesOnLaunch = updatesCheck.Checked
		a.settings.save(a.prefs())
		a.applyTheme() // recolour the whole app immediately
	}, parent)
}

// errInvalidTimeout is the validation error for a bad timeout entry.
var errInvalidTimeout = timeoutError("timeout must be a positive whole number of seconds")

type timeoutError string

func (e timeoutError) Error() string { return string(e) }
