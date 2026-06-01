package ui

import (
	"reflect"
	"testing"

	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/ultramcu/yon/internal/model"
)

func TestFindRuneOffsets(t *testing.T) {
	if got := findRuneOffsets("Hello hello HELLO x", "hello"); !reflect.DeepEqual(got, []int{0, 6, 12}) {
		t.Fatalf("case-insensitive matches = %v, want [0 6 12]", got)
	}
	if got := findRuneOffsets("aaaa", "aa"); !reflect.DeepEqual(got, []int{0, 2}) {
		t.Fatalf("non-overlapping = %v, want [0 2]", got)
	}
	if findRuneOffsets("abc", "") != nil {
		t.Fatal("empty query should yield nil")
	}
	// rune offsets (multi-byte): "ค่า" repeated
	if got := findRuneOffsets("ค่า ค่า", "ค่า"); !reflect.DeepEqual(got, []int{0, 4}) {
		t.Fatalf("unicode rune offsets = %v, want [0 4]", got)
	}
}

func TestRowColOf(t *testing.T) {
	starts := lineStarts("ab\ncd\nef") // line starts at rune 0,3,6
	for _, tc := range []struct{ off, row, col int }{{0, 0, 0}, {1, 0, 1}, {4, 1, 1}, {7, 2, 1}} {
		if r, c := rowColOf(starts, tc.off); r != tc.row || c != tc.col {
			t.Fatalf("rowColOf(%d) = %d,%d want %d,%d", tc.off, r, c, tc.row, tc.col)
		}
	}
}

func TestGridSearch_CountAndNavigate(t *testing.T) {
	test.NewApp()
	grid := widget.NewTextGrid()
	text := "foo bar\nfoo baz\nfoo end"
	grid.SetText(text)
	var g gridSearch
	g.bind(grid, container.NewVScroll(grid), text, func() { grid.SetText(text) })

	if c, total := g.search("foo"); c != 1 || total != 3 {
		t.Fatalf("search foo = %d/%d, want 1/3", c, total)
	}
	if c, _ := g.move(1); c != 2 {
		t.Fatalf("next = %d, want 2", c)
	}
	if c, _ := g.move(-1); c != 1 {
		t.Fatalf("prev = %d, want 1", c)
	}
	if c, _ := g.move(-1); c != 3 {
		t.Fatalf("wrap prev = %d, want 3", c)
	}
	if c, total := g.search("zzz"); c != 0 || total != 0 {
		t.Fatalf("no match = %d/%d, want 0/0", c, total)
	}
}

func TestResponseView_OpenCloseFind(t *testing.T) {
	test.NewApp()
	rv := newResponseView(test.NewWindow(nil))
	rv.setResponse(model.Response{Status: 200, StatusText: "OK", Body: []byte("alpha beta alpha")})

	rv.openFind()
	if !rv.findActive || !rv.find.container.Visible() {
		t.Fatal("openFind should activate + show the bar")
	}
	rv.find.query.SetText("alpha") // triggers onChange → search
	if rv.find.count.Text != "1/2" {
		t.Fatalf("count = %q, want 1/2", rv.find.count.Text)
	}
	rv.closeFind()
	if rv.findActive || rv.find.container.Visible() {
		t.Fatal("closeFind should deactivate + hide the bar")
	}
}
