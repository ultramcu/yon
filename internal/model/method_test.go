package model

import "testing"

// TestMethodConsts pins the string values of the well-known HTTP method consts,
// including the methods Yon added for arbitrary-verb support (PATCH/HEAD/OPTIONS).
func TestMethodConsts(t *testing.T) {
	cases := []struct {
		got  Method
		want string
	}{
		{MethodGet, "GET"},
		{MethodPost, "POST"},
		{MethodPut, "PUT"},
		{MethodDelete, "DELETE"},
		{MethodPatch, "PATCH"},
		{MethodHead, "HEAD"},
		{MethodOptions, "OPTIONS"},
	}
	for _, c := range cases {
		if string(c.got) != c.want {
			t.Errorf("Method const = %q, want %q", c.got, c.want)
		}
	}
}

// TestBodyXMLConst pins the new BodyXML body type's string value.
func TestBodyXMLConst(t *testing.T) {
	if string(BodyXML) != "xml" {
		t.Errorf("BodyXML = %q, want %q", BodyXML, "xml")
	}
}
