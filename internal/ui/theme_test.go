package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"
)

func TestThemeRegistry(t *testing.T) {
	for _, id := range []string{"dark-pro", "warm", "system", "unknown-id"} {
		if themeByID(id) == nil {
			t.Fatalf("themeByID(%q) returned nil", id)
		}
	}
	if themeNames() == nil || len(themeNames()) != len(themeOptions) {
		t.Fatalf("themeNames length mismatch")
	}
	// id<->name round-trips for every registered option
	for _, o := range themeOptions {
		if got := themeIDByName(themeNameByID(o.ID)); got != o.ID {
			t.Fatalf("round-trip failed for %q: got %q", o.ID, got)
		}
	}
	// unknown name/id fall back to the default
	if themeIDByName("Nonexistent") != defaultThemeID {
		t.Fatalf("unknown name should map to default id")
	}
	if themeNameByID("nope") != themeOptions[0].Name {
		t.Fatalf("unknown id should map to default name")
	}
}

func TestSettings_ThemeRoundTrip(t *testing.T) {
	a := New(test.NewApp())

	a.settings.ThemeID = "warm"
	a.settings.save(a.prefs())

	got := loadSettings(a.prefs())
	if got.ThemeID != "warm" {
		t.Fatalf("ThemeID not persisted: %q", got.ThemeID)
	}

	a.settings = got
	a.applyTheme() // must not panic and applies the Warm theme

	// a brand-new app with no stored pref defaults to Dark Pro
	if def := loadSettings(test.NewApp().Preferences()); def.ThemeID != defaultThemeID {
		t.Fatalf("default ThemeID = %q, want %q", def.ThemeID, defaultThemeID)
	}
}
