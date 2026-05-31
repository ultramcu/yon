package yonner

import (
	"context"
	"testing"

	"github.com/ultramcu/yon/internal/model"
)

// TestBuild_QueryOrderPreservedNoMerge guards the fix for url.Values.Encode
// sorting + merging: enabled params must append after the URL's existing query
// in row order, with no alphabetical reordering and no key merging.
func TestBuild_QueryOrderPreservedNoMerge(t *testing.T) {
	req := model.Request{
		Method: model.MethodGet,
		URL:    "http://e.com/?z=1&a=2",
		Params: []model.Param{
			{Key: "a", Value: "new", Enabled: true},
			{Key: "m", Value: "x", Enabled: true},
			{Key: "off", Value: "y"}, // disabled → omitted
		},
	}
	r, err := Build(context.Background(), req, model.Collection{})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := r.URL.RawQuery, "z=1&a=2&a=new&m=x"; got != want {
		t.Fatalf("RawQuery = %q, want %q (order preserved, no sort/merge)", got, want)
	}
}
