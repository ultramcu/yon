package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// Yon ships several selectable appearance themes (chosen in Settings, persisted
// in Preferences). Each is a custom fyne.Theme that overrides the palette; any
// colour/size/font it doesn't set falls back to Fyne's default theme for its
// variant. "Dark Pro" is the default (the dart-logo ink/cyan identity); "Warm"
// is a soft cream alternative; "System" follows Fyne's built-in theme.

// hexc parses "#RRGGBB" into an opaque NRGBA. Invalid input returns transparent.
func hexc(s string) color.NRGBA {
	if len(s) != 7 || s[0] != '#' {
		return color.NRGBA{}
	}
	var r, g, b uint8
	for i, p := range []*uint8{&r, &g, &b} {
		hi, lo := hexNib(s[1+i*2]), hexNib(s[2+i*2])
		*p = hi<<4 | lo
	}
	return color.NRGBA{R: r, G: g, B: b, A: 0xff}
}

func hexNib(c byte) uint8 {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	default:
		return 0
	}
}

func alpha(c color.NRGBA, a uint8) color.NRGBA { c.A = a; return c }

// palette is one theme's colour set plus the base variant used for fallbacks.
type palette struct {
	variant fyne.ThemeVariant
	colors  map[fyne.ThemeColorName]color.Color
}

// yonTheme is a fyne.Theme backed by a palette, delegating fonts/icons/sizes to
// the default theme (only colours are themed in v1).
type yonTheme struct{ p palette }

func (t yonTheme) Color(n fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	if c, ok := t.p.colors[n]; ok {
		return c
	}
	return theme.DefaultTheme().Color(n, t.p.variant)
}

func (t yonTheme) Font(s fyne.TextStyle) fyne.Resource     { return theme.DefaultTheme().Font(s) }
func (t yonTheme) Icon(n fyne.ThemeIconName) fyne.Resource { return theme.DefaultTheme().Icon(n) }
func (t yonTheme) Size(n fyne.ThemeSizeName) float32       { return theme.DefaultTheme().Size(n) }

// darkProPalette is the v2 "Dark Pro" identity: deep ink navy + cyan/indigo.
var darkProPalette = palette{
	variant: theme.VariantDark,
	colors: map[fyne.ThemeColorName]color.Color{
		theme.ColorNameBackground:        hexc("#0E1726"),
		theme.ColorNameForeground:        hexc("#D7E2F2"),
		theme.ColorNamePrimary:           hexc("#18C5E8"),
		theme.ColorNameButton:            hexc("#13203A"),
		theme.ColorNameDisabledButton:    hexc("#162741"),
		theme.ColorNameInputBackground:   hexc("#0B1320"),
		theme.ColorNameInputBorder:       hexc("#22324F"),
		theme.ColorNameSeparator:         hexc("#1A2940"),
		theme.ColorNameMenuBackground:    hexc("#101D33"),
		theme.ColorNameOverlayBackground: hexc("#101D33"),
		theme.ColorNameHover:             hexc("#162741"),
		theme.ColorNamePlaceHolder:       hexc("#7C8DA8"),
		theme.ColorNameDisabled:          hexc("#5A6E8C"),
		theme.ColorNameSelection:         alpha(hexc("#18C5E8"), 0x40),
		theme.ColorNameShadow:            alpha(color.NRGBA{}, 0x80),
		theme.ColorNameError:             hexc("#F26D6D"),
		theme.ColorNameSuccess:           hexc("#34D399"),
		theme.ColorNameWarning:           hexc("#F2B441"),
	},
}

// warmPalette is the v4 "Warm" identity: cream surfaces + coral accent.
var warmPalette = palette{
	variant: theme.VariantLight,
	colors: map[fyne.ThemeColorName]color.Color{
		theme.ColorNameBackground:        hexc("#FBF7F1"),
		theme.ColorNameForeground:        hexc("#1B2436"),
		theme.ColorNamePrimary:           hexc("#FF6B5E"),
		theme.ColorNameButton:            hexc("#FFFDFA"),
		theme.ColorNameDisabledButton:    hexc("#F1ECE4"),
		theme.ColorNameInputBackground:   hexc("#FFFFFF"),
		theme.ColorNameInputBorder:       hexc("#ECE6DC"),
		theme.ColorNameSeparator:         hexc("#ECE6DC"),
		theme.ColorNameMenuBackground:    hexc("#FFFDFA"),
		theme.ColorNameOverlayBackground: hexc("#FFFDFA"),
		theme.ColorNameHover:             hexc("#F3ECE2"),
		theme.ColorNamePlaceHolder:       hexc("#8A93A3"),
		theme.ColorNameDisabled:          hexc("#B6BCC6"),
		theme.ColorNameSelection:         alpha(hexc("#FF6B5E"), 0x2E),
		theme.ColorNameError:             hexc("#E5484D"),
		theme.ColorNameSuccess:           hexc("#1FB877"),
		theme.ColorNameWarning:           hexc("#FFB454"),
	},
}

// ThemeOption is a selectable appearance entry shown in Settings.
type ThemeOption struct {
	ID    string
	Name  string
	Theme fyne.Theme
}

// themeOptions is the ordered list of selectable themes; the first is the default.
var themeOptions = []ThemeOption{
	{ID: "dark-pro", Name: "Dark Pro", Theme: yonTheme{darkProPalette}},
	{ID: "warm", Name: "Warm", Theme: yonTheme{warmPalette}},
	{ID: "system", Name: "System", Theme: theme.DefaultTheme()},
}

const defaultThemeID = "dark-pro"

// themeByID returns the fyne.Theme for id, falling back to the default theme.
func themeByID(id string) fyne.Theme {
	for _, o := range themeOptions {
		if o.ID == id {
			return o.Theme
		}
	}
	return themeOptions[0].Theme
}

// themeNames / id<->name helpers for the Settings Select.
func themeNames() []string {
	names := make([]string, len(themeOptions))
	for i, o := range themeOptions {
		names[i] = o.Name
	}
	return names
}

func themeNameByID(id string) string {
	for _, o := range themeOptions {
		if o.ID == id {
			return o.Name
		}
	}
	return themeOptions[0].Name
}

func themeIDByName(name string) string {
	for _, o := range themeOptions {
		if o.Name == name {
			return o.ID
		}
	}
	return defaultThemeID
}
