package ui

import (
	"image/color"

	"github.com/ultramcu/yon/internal/model"
)

// Shared visual tokens for the v2 "Dark Pro" layout, so the sidebar, request
// bar, and response panel all use the SAME accent and HTTP-method colours.
// (Status-class colours live in responseview.go as colorStatus2xx etc.)

// Method colours — used for the sidebar verb chips and method pill. Matches the
// v2 mockup: GET cyan, POST green, PUT amber, DELETE red; PATCH violet, HEAD
// teal, OPTIONS blue; any other/unknown verb falls back to slate.
var methodColors = map[model.Method]color.Color{
	model.MethodGet:     color.NRGBA{R: 0x18, G: 0xC5, B: 0xE8, A: 0xff}, // cyan
	model.MethodPost:    color.NRGBA{R: 0x34, G: 0xD3, B: 0x99, A: 0xff}, // green
	model.MethodPut:     color.NRGBA{R: 0xF2, G: 0xB4, B: 0x41, A: 0xff}, // amber
	model.MethodDelete:  color.NRGBA{R: 0xF2, G: 0x6D, B: 0x6D, A: 0xff}, // red
	model.MethodPatch:   color.NRGBA{R: 0xB4, G: 0x8E, B: 0xF0, A: 0xff}, // violet
	model.MethodHead:    color.NRGBA{R: 0x4F, G: 0xD1, B: 0xC5, A: 0xff}, // teal
	model.MethodOptions: color.NRGBA{R: 0x6E, G: 0x9B, B: 0xF0, A: 0xff}, // blue
}

// methodColorSlate is the fallback verb colour for any other/unknown method.
var methodColorSlate = color.NRGBA{R: 0x8A, G: 0xA0, B: 0xC2, A: 0xff}

// methodColor returns the accent colour for an HTTP method (slate fallback).
func methodColor(m model.Method) color.Color {
	if c, ok := methodColors[m]; ok {
		return c
	}
	return methodColorSlate
}
