package ui

import (
	"context"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/ultramcu/yon/internal/model"
	"github.com/ultramcu/yon/internal/yonner"
)

// methodOptions lists the v1 HTTP methods for the Method Select.
var methodOptions = []string{
	string(model.MethodGet),
	string(model.MethodPost),
	string(model.MethodPut),
	string(model.MethodDelete),
}

// bodyTypeOptions lists the Body-type Select values, label → model.BodyType.
var bodyTypeLabels = []string{"None", "JSON", "Text"}

func bodyTypeFromLabel(label string) model.BodyType {
	switch label {
	case "JSON":
		return model.BodyJSON
	case "Text":
		return model.BodyText
	default:
		return model.BodyNone
	}
}

func bodyTypeToLabel(t model.BodyType) string {
	switch t {
	case model.BodyJSON:
		return "JSON"
	case model.BodyText:
		return "Text"
	default:
		return "None"
	}
}

// requestTab is one open Request editor living in the window's DocTabs. It edits
// the Request at Collection index idx, commits edits back via the window, runs
// Send off the UI goroutine (one in-flight send at a time), and renders the
// Response into a read-only viewer.
type requestTab struct {
	win *Window
	idx int

	tab *container.TabItem

	// editing widgets
	nameEntry   *widget.Entry
	methodSel   *widget.Select
	urlEntry    *widget.Entry
	paramsTable *kvTable
	headerTable *kvTable
	authEditor  *authEditor
	bodyTypeSel *widget.Select
	bodyEntry   *widget.Entry

	sendBtn *widget.Button

	// sub-tab items kept so their count badges ("Params 3"/"Headers 1") can be
	// updated when rows change.
	paramsTab *container.TabItem
	headerTab *container.TabItem
	subTabs   *container.AppTabs

	response *responseView

	// dirty marks this tab as having unsaved edits — shows a ● on its DocTab.
	dirty bool
	// lastResp is the most recent successful Response (nil if none/error), shown
	// in the window status bar when this tab is the active one.
	lastResp *model.Response

	// in-flight send state: cancel is non-nil while a send is running. sendSeq is
	// a generation token bumped on every start and every cancel, so a returning
	// goroutine can tell whether its result is still the live one (a fast
	// cancel→resend must not let the stale send's result clobber the new one).
	cancel  context.CancelFunc
	sendSeq uint64
}

// newRequestTab builds an editor Tab for the Request at idx.
func newRequestTab(w *Window, idx int) *requestTab {
	rt := &requestTab{win: w, idx: idx}
	req := w.coll.Requests[idx]

	rt.nameEntry = widget.NewEntry()
	rt.nameEntry.SetPlaceHolder("Request name (optional)")
	rt.nameEntry.SetText(req.Name)
	rt.nameEntry.OnChanged = func(string) { rt.commit() }

	// Build the Select with no handler, set its initial value, then wire OnChanged —
	// otherwise SetSelected fires commit() before the Param/Header/Auth/Body widgets
	// below exist, dereferencing nil (panics on tab construction).
	rt.methodSel = widget.NewSelect(methodOptions, nil)
	if req.Method == "" {
		rt.methodSel.SetSelected(string(model.MethodGet))
	} else {
		rt.methodSel.SetSelected(string(req.Method))
	}
	rt.methodSel.OnChanged = func(string) { rt.commit() }

	rt.urlEntry = widget.NewEntry()
	rt.urlEntry.SetPlaceHolder("https://api.example.com/path")
	rt.urlEntry.SetText(req.URL)
	rt.urlEntry.OnChanged = func(string) { rt.commit() }
	// Enter in the URL field sends the request (unless one is already in flight).
	rt.urlEntry.OnSubmitted = func(string) {
		if rt.cancel == nil {
			rt.startSend()
		}
	}

	rt.sendBtn = widget.NewButtonWithIcon("Send", theme.MailSendIcon(), rt.onSend)
	rt.sendBtn.Importance = widget.HighImportance

	// Top bar: [Method ▾][URL.................][✈ Send] — a clean compact bar; the
	// Method Select renders as a "GET ▾" pill and Send is the primary (cyan) action.
	topBar := container.NewBorder(nil, nil, rt.methodSel, rt.sendBtn, rt.urlEntry)

	rt.paramsTable = newKVTable(req.Params, func() { rt.commit() })
	rt.headerTable = newKVTable(req.Headers, func() { rt.commit() })
	rt.authEditor = newAuthEditor(req.Auth, true, func() { rt.commit() })
	bodyPane := rt.buildBody(req)

	rt.paramsTab = container.NewTabItem("Params", rt.paramsTable.container)
	rt.headerTab = container.NewTabItem("Headers", rt.headerTable.container)
	rt.subTabs = container.NewAppTabs(
		rt.paramsTab,
		rt.headerTab,
		container.NewTabItem("Auth", container.NewVScroll(rt.authEditor.container)),
		container.NewTabItem("Body", bodyPane),
	)
	rt.refreshTabBadges() // initial counts from the seeded Request

	editorTop := container.NewVBox(
		rt.nameEntry,
		topBar,
	)
	requestPane := container.NewBorder(editorTop, nil, nil, nil, rt.subTabs)

	rt.response = newResponseView(w.win)

	// Request editor on top, Response below, resizable.
	split := container.NewVSplit(requestPane, rt.response.container)
	split.SetOffset(0.5)

	rt.tab = container.NewTabItem(tabTitle(req), split)
	return rt
}

// buildBody constructs the Body sub-tab: a type Select plus a multiline Entry
// (editing input may use an Entry per ADR-0001). The editor is hidden for None.
func (rt *requestTab) buildBody(req model.Request) fyne.CanvasObject {
	rt.bodyEntry = widget.NewMultiLineEntry()
	rt.bodyEntry.SetText(req.Body.Content)
	rt.bodyEntry.OnChanged = func(string) { rt.commit() }
	rt.bodyEntry.Wrapping = fyne.TextWrapOff

	rt.bodyTypeSel = widget.NewSelect(bodyTypeLabels, func(label string) {
		if label == "None" {
			rt.bodyEntry.Hide()
		} else {
			rt.bodyEntry.Show()
		}
		rt.commit()
	})
	rt.bodyTypeSel.SetSelected(bodyTypeToLabel(req.Body.Type))
	if req.Body.Type == model.BodyNone || req.Body.Type == "" {
		rt.bodyEntry.Hide()
	}

	return container.NewBorder(
		container.NewHBox(widget.NewLabel("Body type:"), rt.bodyTypeSel),
		nil, nil, nil,
		rt.bodyEntry,
	)
}

// current reads the editor widgets back into a model.Request.
func (rt *requestTab) current() model.Request {
	return model.Request{
		Name:    rt.nameEntry.Text,
		Method:  model.Method(rt.methodSel.Selected),
		URL:     rt.urlEntry.Text,
		Params:  rt.paramsTable.value(),
		Headers: rt.headerTable.value(),
		Auth:    rt.authEditor.value(),
		Body: model.Body{
			Type:    bodyTypeFromLabel(rt.bodyTypeSel.Selected),
			Content: rt.bodyEntry.Text,
		},
	}
}

// commit writes the editor's Request back into the Collection (marks dirty,
// updates sidebar + tab title) and refreshes the Params/Headers count badges.
func (rt *requestTab) commit() {
	rt.dirty = true
	rt.refreshTabBadges()
	rt.win.commitRequest(rt.idx, rt.current())
	rt.win.updateStatusBar()
}

// enabledParamCount counts the rows that are both enabled and have content
// (key or value present) — i.e. the params/headers that will actually be sent.
func enabledParamCount(ps []model.Param) int {
	n := 0
	for _, p := range ps {
		if p.Enabled && (p.Key != "" || p.Value != "") {
			n++
		}
	}
	return n
}

// tabBadge renders a sub-tab title with a trailing count when non-empty, e.g.
// "Params 3"; an empty/zero count shows just the base label ("Params").
func tabBadge(base string, n int) string {
	if n <= 0 {
		return base
	}
	return fmt.Sprintf("%s %d", base, n)
}

// refreshTabBadges updates the Params/Headers sub-tab titles with the current
// enabled+present row counts. Safe to call before subTabs exists (no-op).
func (rt *requestTab) refreshTabBadges() {
	if rt.paramsTab == nil || rt.headerTab == nil {
		return
	}
	rt.paramsTab.Text = tabBadge("Params", enabledParamCount(rt.paramsTable.value()))
	rt.headerTab.Text = tabBadge("Headers", enabledParamCount(rt.headerTable.value()))
	if rt.subTabs != nil {
		rt.subTabs.Refresh()
	}
}

// onSend toggles between starting a send and cancelling the in-flight one.
func (rt *requestTab) onSend() {
	if rt.cancel != nil {
		rt.cancelInFlight()
		return
	}
	rt.startSend()
}

// startSend launches a send off the UI goroutine. One in-flight send per Tab:
// the button flips to "Cancel" until the send completes or is cancelled.
func (rt *requestTab) startSend() {
	req := rt.current()
	coll := rt.win.coll
	opts := rt.win.app.settings.options()

	ctx, cancel := context.WithCancel(context.Background())
	rt.sendSeq++
	seq := rt.sendSeq
	rt.cancel = cancel
	rt.setSending(true)
	rt.response.setPending()

	go func() {
		resp, err := yonner.Send(ctx, req, coll, opts)
		// Marshal the result back onto the UI goroutine (Fyne 2.7 requires UI
		// updates on the main goroutine — fyne.Do schedules this safely).
		fyne.Do(func() {
			// Ignore a result that a later cancel or a newer send has superseded.
			if rt.sendSeq != seq {
				return
			}
			rt.cancel = nil
			rt.setSending(false)
			if err != nil {
				rt.lastResp = nil
				rt.response.setError(err)
				rt.win.updateStatusBar()
				return
			}
			r := resp
			rt.lastResp = &r
			rt.response.setResponse(resp)
			rt.win.updateStatusBar()
		})
	}()
}

// cancelInFlight cancels a running send (if any) and resets the button. Safe to
// call when nothing is in flight.
func (rt *requestTab) cancelInFlight() {
	if rt.cancel == nil {
		return
	}
	rt.cancel()
	rt.cancel = nil
	rt.sendSeq++ // invalidate the in-flight send's pending result
	rt.setSending(false)
}

// setSending switches the Send button between "Send" and "Cancel".
func (rt *requestTab) setSending(sending bool) {
	if sending {
		rt.sendBtn.SetText("Cancel")
		rt.sendBtn.SetIcon(theme.CancelIcon())
		rt.sendBtn.Importance = widget.DangerImportance
	} else {
		rt.sendBtn.SetText("Send")
		rt.sendBtn.SetIcon(theme.MailSendIcon())
		rt.sendBtn.Importance = widget.HighImportance
	}
	rt.sendBtn.Refresh()
}
