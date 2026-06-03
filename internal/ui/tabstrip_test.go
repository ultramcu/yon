package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/ultramcu/yon/internal/model"
)

// TestTabStrip_AppendSelectSelected verifies that Append registers cards and
// Select makes a given card the current selection, observable via Selected().
func TestTabStrip_AppendSelectSelected(t *testing.T) {
	test.NewApp()
	s := newTabStrip()
	first := newTabCard(widget.NewLabel("a"))
	second := newTabCard(widget.NewLabel("b"))
	s.Append(first)
	s.Append(second)

	s.Select(first)
	if s.Selected() != first {
		t.Fatalf("Selected() = %p, want first %p", s.Selected(), first)
	}
	s.Select(second)
	if s.Selected() != second {
		t.Fatalf("Selected() = %p, want second %p", s.Selected(), second)
	}
}

// TestTabStrip_OnSelectedFires verifies Select fires the OnSelected callback
// with the card that was selected.
func TestTabStrip_OnSelectedFires(t *testing.T) {
	test.NewApp()
	s := newTabStrip()
	c := newTabCard(widget.NewLabel("a"))
	s.Append(c)

	var got *tabCard
	s.OnSelected = func(card *tabCard) { got = card }

	s.Select(c)
	if got != c {
		t.Fatalf("OnSelected received %p, want %p", got, c)
	}
}

// TestTabStrip_RemoveSelectedSelectsNeighbour verifies that removing the
// currently-selected card promotes a remaining neighbour to selected, so
// Selected() stays non-nil while cards remain and is never the removed card.
func TestTabStrip_RemoveSelectedSelectsNeighbour(t *testing.T) {
	test.NewApp()
	s := newTabStrip()
	a := newTabCard(widget.NewLabel("a"))
	b := newTabCard(widget.NewLabel("b"))
	c := newTabCard(widget.NewLabel("c"))
	s.Append(a)
	s.Append(b)
	s.Append(c)

	s.Select(b)
	s.Remove(b)

	sel := s.Selected()
	if sel == nil {
		t.Fatal("Selected() = nil after removing selected with cards remaining, want a neighbour")
	}
	if sel == b {
		t.Fatal("Selected() still points to the removed card b")
	}
	if sel != a && sel != c {
		t.Fatalf("Selected() = %p, want one of remaining cards a %p or c %p", sel, a, c)
	}
}

// TestTabStrip_RemoveLastSelectsNil verifies that removing the only (selected)
// card leaves an empty strip with Selected()==nil.
func TestTabStrip_RemoveLastSelectsNil(t *testing.T) {
	test.NewApp()
	s := newTabStrip()
	c := newTabCard(widget.NewLabel("only"))
	s.Append(c)
	s.Select(c)

	s.Remove(c)
	if s.Selected() != nil {
		t.Fatalf("Selected() = %p after removing last card, want nil", s.Selected())
	}
}

// TestTabCard_SetRequestNoPanicStillSelectable verifies setRequest does not
// panic and the card remains usable (appendable + selectable) afterwards.
func TestTabCard_SetRequestNoPanicStillSelectable(t *testing.T) {
	test.NewApp()
	c := newTabCard(widget.NewLabel("content"))
	c.setRequest(model.MethodPost, "Create user", true)

	s := newTabStrip()
	s.Append(c)
	s.Select(c)
	if s.Selected() != c {
		t.Fatalf("Selected() = %p after setRequest+select, want %p", s.Selected(), c)
	}
}

// TestTabStrip_RemoveNonSelectedKeepsSelection verifies that removing a card
// that is not the current selection leaves the selection unchanged.
func TestTabStrip_RemoveNonSelectedKeepsSelection(t *testing.T) {
	test.NewApp()
	s := newTabStrip()
	a := newTabCard(widget.NewLabel("a"))
	b := newTabCard(widget.NewLabel("b"))
	s.Append(a)
	s.Append(b)

	s.Select(a)
	s.Remove(b)
	if s.Selected() != a {
		t.Fatalf("Selected() = %p after removing non-selected b, want a %p", s.Selected(), a)
	}
}
