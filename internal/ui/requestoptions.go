package ui

import (
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/ultramcu/yon/internal/model"
)

// Option-Select label strings. These are the contract between the widgets and
// the pure helpers (requestOptionsFromControls / seedOptionControls): the
// widgets are built with these exact slices and the helpers switch on these
// exact strings. The first entry of each ("Default (global)") is the tri-state
// "inherit the global Setting" choice that maps to a nil field.
const (
	optDefault = "Default (global)"

	optFollow   = "Follow"
	optNoFollow = "Don't follow"

	optTLSAllow  = "Allow (insecure)"
	optTLSVerify = "Verify (secure)"
)

// followOptions / tlsOptions are the Select option lists. "Default (global)"
// leads each so a fresh, never-overridden Request shows the inherit choice.
var (
	followOptions = []string{optDefault, optFollow, optNoFollow}
	tlsOptions    = []string{optDefault, optTLSAllow, optTLSVerify}
)

// timeoutPlaceholder hints that a blank Timeout entry inherits the global
// Settings timeout rather than meaning "no timeout".
const timeoutPlaceholder = "Default (global)"

// requestOptionsTab is the "Options" sub-tab of a request editor: per-request
// overrides for Timeout, Follow-redirects, and Insecure-TLS. Each control is
// tri-state — its "Default (global)" / blank choice inherits the corresponding
// app-level Setting (see settings.go), and any other choice overrides it for
// this one Request.
//
// Dev A (requesteditor.go) owns the wiring: requestTab gains an optionsTab
// field, newRequestTab calls newRequestOptionsTab + segTabs.Append("Options",
// …), and current() folds value() into the model.Request. This type only
// builds the widgets, seeds them from seed.Options, and exposes value().
type requestOptionsTab struct {
	// container is the built tab content, appended into the request editor's
	// segmented sub-tabs by Dev A.
	container fyne.CanvasObject

	timeoutEntry *widget.Entry
	followSel    *widget.Select
	tlsSel       *widget.Select
}

// newRequestOptionsTab builds the Options tab UI from seed.Options and wires
// every control's OnChanged to rt.commit() so an edit marks the tab dirty and
// persists, exactly like the Params/Headers/Auth/Body tabs. A nil seed.Options
// (the backward-compatible default for a Request that predates per-request
// options) seeds every control to its "Default (global)" / blank inherit state.
//
// rt is the owning *requestTab (defined in requesteditor.go); this function
// only calls its commit() method and never mutates its fields.
func newRequestOptionsTab(rt *requestTab, seed model.Request) *requestOptionsTab {
	o := &requestOptionsTab{}

	timeoutText, followSel, tlsSel := seedOptionControls(seed.Options)

	// Timeout: a numeric Entry. Blank inherits the global timeout; a value ≥ 0
	// overrides it, where 0 means "no timeout" explicitly (see
	// requestOptionsFromControls). SetText before wiring OnChanged so the seed
	// does not fire a construction-time commit (which would flip rt.dirty on a
	// freshly opened tab) — matching the methodSel idiom in newRequestTab.
	o.timeoutEntry = widget.NewEntry()
	o.timeoutEntry.SetPlaceHolder(timeoutPlaceholder)
	o.timeoutEntry.SetText(timeoutText)
	o.timeoutEntry.OnChanged = func(string) { rt.commit() }

	// Follow redirects: Default(nil) / Follow(&true) / Don't follow(&false).
	o.followSel = widget.NewSelect(followOptions, nil)
	o.followSel.SetSelected(followSel)
	o.followSel.OnChanged = func(string) { rt.commit() }

	// Allow insecure TLS: Default(nil) / Allow(&true) / Verify(&false).
	o.tlsSel = widget.NewSelect(tlsOptions, nil)
	o.tlsSel.SetSelected(tlsSel)
	o.tlsSel.OnChanged = func(string) { rt.commit() }

	form := widget.NewForm(
		widget.NewFormItem("Timeout (seconds)", o.timeoutEntry),
		widget.NewFormItem("Follow redirects", o.followSel),
		widget.NewFormItem("Allow insecure TLS", o.tlsSel),
	)

	o.container = container.NewVScroll(form)
	return o
}

// value reads the live controls back into a *model.RequestOptions, returning
// nil when NOTHING is overridden (all controls Default/blank) so an untouched
// Options tab leaves Request.Options nil — preserving backward compatibility
// for Requests that never set any override.
func (o *requestOptionsTab) value() *model.RequestOptions {
	return requestOptionsFromControls(o.timeoutEntry.Text, o.followSel.Selected, o.tlsSel.Selected)
}

// requestOptionsFromControls maps raw control values to the tri-state
// model.RequestOptions. It is Fyne-free so it can be unit-tested directly.
//
// Mapping (each independent):
//   - timeoutText: blank or unparseable/negative → inherit (nil); an integer
//     ≥ 0 → &n, where 0 means "no timeout" explicitly.
//   - followSel: optDefault/"" → nil; optFollow → &true; optNoFollow → &false.
//   - tlsSel:     optDefault/"" → nil; optTLSAllow → &true; optTLSVerify → &false.
//
// Returns nil when every field resolves to nil, so "all Default/blank" yields no
// override struct at all.
func requestOptionsFromControls(timeoutText, followSel, tlsSel string) *model.RequestOptions {
	ro := &model.RequestOptions{
		TimeoutSeconds:  parseTimeoutOverride(timeoutText),
		FollowRedirects: triFromSelect(followSel, optFollow, optNoFollow),
		InsecureTLS:     triFromSelect(tlsSel, optTLSAllow, optTLSVerify),
	}
	if ro.TimeoutSeconds == nil && ro.FollowRedirects == nil && ro.InsecureTLS == nil {
		return nil
	}
	return ro
}

// seedOptionControls is the inverse of requestOptionsFromControls: it turns a
// loaded *model.RequestOptions back into the raw control values used to seed the
// widgets. A nil ro (no overrides) yields blank/Default for every control. It is
// Fyne-free for direct unit testing and round-trip checks.
func seedOptionControls(ro *model.RequestOptions) (timeoutText, followSel, tlsSel string) {
	if ro == nil {
		return "", optDefault, optDefault
	}
	if ro.TimeoutSeconds != nil {
		timeoutText = strconv.Itoa(*ro.TimeoutSeconds)
	}
	followSel = triToSelect(ro.FollowRedirects, optFollow, optNoFollow)
	tlsSel = triToSelect(ro.InsecureTLS, optTLSAllow, optTLSVerify)
	return timeoutText, followSel, tlsSel
}

// parseTimeoutOverride parses the Timeout entry into a *int override: blank or
// unparseable or negative → nil (inherit the global timeout); an integer ≥ 0 →
// &n (0 meaning an explicit "no timeout").
func parseTimeoutOverride(text string) *int {
	n, err := strconv.Atoi(text)
	if err != nil || n < 0 {
		return nil
	}
	return &n
}

// triFromSelect maps a Select value to a tri-state *bool: optDefault or "" →
// nil; trueLabel → &true; falseLabel → &false. An unrecognised value is treated
// as Default (nil) so a corrupt/unknown selection inherits rather than guessing.
func triFromSelect(sel, trueLabel, falseLabel string) *bool {
	switch sel {
	case trueLabel:
		t := true
		return &t
	case falseLabel:
		f := false
		return &f
	default: // optDefault, "", or anything unexpected
		return nil
	}
}

// triToSelect is the inverse of triFromSelect: nil → optDefault; &true →
// trueLabel; &false → falseLabel.
func triToSelect(b *bool, trueLabel, falseLabel string) string {
	if b == nil {
		return optDefault
	}
	if *b {
		return trueLabel
	}
	return falseLabel
}
