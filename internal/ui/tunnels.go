package ui

import (
	"fmt"
	"sort"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/ultramcu/yon/internal/tunnel"
)

// tappable wraps a single CanvasObject (the footer jump-host indicator) to make
// it clickable, since canvas.Text is not itself tappable. It forwards a Tap to
// the onTap callback (opening the Tunnels window) and otherwise renders its
// child verbatim, so it costs nothing layout-wise in the footer HBox.
type tappable struct {
	widget.BaseWidget
	child fyne.CanvasObject
	onTap func()
}

func newTappable(child fyne.CanvasObject, onTap func()) *tappable {
	t := &tappable{child: child, onTap: onTap}
	t.ExtendBaseWidget(t)
	return t
}

func (t *tappable) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(t.child)
}

// Tapped opens the Tunnels window (fyne.Tappable).
func (t *tappable) Tapped(*fyne.PointEvent) {
	if t.onTap != nil {
		t.onTap()
	}
}

// tunnelTableHeaders labels the Tunnels window columns. Kept beside
// tunnelStatusRows so the column order stays in lock-step with the cell order
// that helper emits.
var tunnelTableHeaders = []string{"Jump host", "State", "Refs", "Uptime", "Error"}

// openTunnelsWindow opens (or focuses) the Tunnels window: a live table of the
// app's SSH jump-host tunnels — one row per Tunnel with its display address,
// state, refcount, uptime and last error — plus a Disconnect control. It is
// reachable from Collection ▸ Tunnels… and from the footer tunnel indicator.
//
// Connect is intentionally not offered here: Status() exposes only the display
// address, not the full model.JumpHost needed to dial, and the core already
// reconnects lazily on the next send. The window therefore supports Disconnect
// (by Tunnel ID) plus live status, and a hint that sending reconnects.
func (a *App) openTunnelsWindow(parent *Window) {
	if a.tunnels == nil {
		return
	}

	win := a.fyneApp.NewWindow("Tunnels")

	var rows [][]string
	var selected int = -1

	table := widget.NewTable(
		func() (int, int) { return len(rows) + 1, len(tunnelTableHeaders) },
		func() fyne.CanvasObject {
			l := widget.NewLabel("")
			l.Truncation = fyne.TextTruncateEllipsis
			return l
		},
		func(id widget.TableCellID, o fyne.CanvasObject) {
			l := o.(*widget.Label)
			if id.Row == 0 {
				l.TextStyle = fyne.TextStyle{Bold: true}
				l.SetText(tunnelTableHeaders[id.Col])
				return
			}
			l.TextStyle = fyne.TextStyle{}
			r := id.Row - 1
			if r >= 0 && r < len(rows) && id.Col < len(rows[r]) {
				l.SetText(rows[r][id.Col])
			} else {
				l.SetText("")
			}
		},
	)
	table.OnSelected = func(id widget.TableCellID) {
		if id.Row >= 1 {
			selected = id.Row - 1
		} else {
			selected = -1
		}
	}

	// sts caches the current snapshot so the Disconnect button can map the
	// selected display row back to a Tunnel ID. Kept in sync with rows.
	var sts []tunnel.TunnelStatus

	disconnectBtn := widget.NewButtonWithIcon("Disconnect", theme.CancelIcon(), func() {
		if selected < 0 || selected >= len(sts) {
			return
		}
		a.tunnels.Disconnect(sts[selected].ID)
	})

	empty := widget.NewLabel("No tunnels. A jump host opens one on the next send.")
	empty.Alignment = fyne.TextAlignCenter

	body := container.NewStack(table, empty)

	refresh := func() {
		sts = a.tunnels.Status()
		rows = tunnelStatusRows(sts, time.Now())
		if len(rows) == 0 {
			empty.Show()
		} else {
			empty.Hide()
		}
		// Set sensible column widths once data is present.
		table.SetColumnWidth(0, 220)
		table.SetColumnWidth(1, 110)
		table.SetColumnWidth(2, 70)
		table.SetColumnWidth(3, 90)
		table.SetColumnWidth(4, 260)
		table.Refresh()
	}
	refresh()

	hint := widget.NewLabel("Disconnect tears a tunnel down; the next send reconnects it.")
	hint.TextStyle = fyne.TextStyle{Italic: true}

	controls := container.NewHBox(
		disconnectBtn,
		widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), refresh),
	)

	content := container.NewBorder(nil, container.NewVBox(hint, controls), nil, nil, body)
	win.SetContent(content)
	win.Resize(fyne.NewSize(760, 360))

	// Live updates: re-read Status on every manager change, marshalled onto the
	// Fyne goroutine. Subscribe fires from arbitrary goroutines, so the refresh
	// must go through fyne.Do.
	unsub := a.tunnels.Subscribe(func() { fyne.Do(refresh) })
	win.SetOnClosed(func() { unsub() })

	win.Show()
}

// tunnelStatusRows maps a tunnel status snapshot to display cells — one inner
// slice per Tunnel, columns [JumpHost, State, "Used by N", uptime, Err] — for
// the Tunnels table. It is Fyne-free and stable-sorted by JumpHost (then ID) so
// the blind tester can verify the mapping and ordering without a GUI. now is
// injected so uptime is deterministic in tests.
func tunnelStatusRows(sts []tunnel.TunnelStatus, now time.Time) [][]string {
	sorted := make([]tunnel.TunnelStatus, len(sts))
	copy(sorted, sts)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].JumpHost != sorted[j].JumpHost {
			return sorted[i].JumpHost < sorted[j].JumpHost
		}
		return sorted[i].ID < sorted[j].ID
	})

	out := make([][]string, 0, len(sorted))
	for _, s := range sorted {
		out = append(out, []string{
			s.JumpHost,
			s.State.String(),
			fmt.Sprintf("Used by %d", s.RefCount),
			tunnelUptime(s.Since, now),
			s.Err,
		})
	}
	return out
}

// tunnelUptime renders the time a Tunnel has spent in its current state as a
// compact "1h2m" / "3m4s" / "5s" string — how long it has held that state since
// the Since timestamp. Blank only when Since is unset (a never-touched tunnel).
// Sub-second or negative durations show as "0s".
func tunnelUptime(since, now time.Time) string {
	if since.IsZero() {
		return ""
	}
	d := now.Sub(since)
	if d < time.Second {
		return "0s"
	}
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	switch {
	case h > 0:
		return fmt.Sprintf("%dh%dm", h, m)
	case m > 0:
		return fmt.Sprintf("%dm%ds", m, s)
	default:
		return fmt.Sprintf("%ds", s)
	}
}
