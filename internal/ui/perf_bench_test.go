package ui

import (
	"fmt"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"
)

// bigJSON returns a JSON array body roughly targetBytes large.
func bigJSON(targetBytes int) []byte {
	var b strings.Builder
	b.WriteString(`{"count":0,"items":[`)
	for i := 0; b.Len() < targetBytes; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"id":%d,"name":"item-%05d","value":%d,"note":"lorem ipsum dolor sit amet"}`, i, i, i*7)
	}
	b.WriteString("]}")
	return []byte(b.String())
}

// BenchmarkRenderLargeBody measures the cost of rendering a large JSON response
// body into the read-only TextGrid with Pretty syntax colouring — the path the
// user reported as laggy. Runs on the Fyne test driver (CPU cost of SetText +
// styleTextGridJSON is driver-independent).
func BenchmarkRenderLargeBody(b *testing.B) {
	test.NewApp()
	w := test.NewWindow(nil)
	rv := newResponseView(w)
	body := bigJSON(maxDisplayBytes) // ~256 KB, the display cap

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rv.fullBody = body
		rv.renderBody()
	}
}

// BenchmarkRenderMediumBody is a smaller reference point (~32 KB).
func BenchmarkRenderMediumBody(b *testing.B) {
	test.NewApp()
	w := test.NewWindow(nil)
	rv := newResponseView(w)
	body := bigJSON(32 * 1024)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rv.fullBody = body
		rv.renderBody()
	}
}
