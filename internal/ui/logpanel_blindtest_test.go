package ui

import (
	"strings"
	"testing"
	"time"
)

// Blind tests for the HTTP request log formatter (issue #30), written from the
// contract only. They depend solely on the contract symbols (logEntry,
// formatLogEntry, formatLog) plus the same-package helpers formatDuration /
// formatSize, which are called to build expectations rather than hardcoded.

// fixed, deterministic time -> "15:04:05"
func blindFixedTime() time.Time {
	return time.Date(2026, 6, 5, 15, 4, 5, 0, time.UTC)
}

func TestFormatLogEntry_Success(t *testing.T) {
	tm := blindFixedTime()
	e := logEntry{
		Time:       tm,
		Name:       "Ping",
		Method:     "GET",
		URL:        "http://h/json",
		Status:     200,
		StatusText: "OK",
		Duration:   3 * time.Millisecond,
		Size:       1234,
	}
	got := formatLogEntry(e)

	wantDur := formatDuration(3 * time.Millisecond)
	wantSize := formatSize(1234)

	for _, sub := range []string{
		"15:04:05",
		"[Ping]",
		"GET http://h/json",
		"→ 200 OK",
		wantDur,
		wantSize,
	} {
		if !strings.Contains(got, sub) {
			t.Errorf("formatLogEntry success: expected substring %q in %q", sub, got)
		}
	}

	// structure: a size clause hangs off the status line via " · "
	if !strings.Contains(got, "→ 200 OK ·") {
		t.Errorf("formatLogEntry success: expected %q structure in %q", "→ 200 OK ·", got)
	}
	// must not be an error line
	if strings.Contains(got, "ERROR:") {
		t.Errorf("formatLogEntry success: unexpected ERROR clause in %q", got)
	}
}

func TestFormatLogEntry_NoSizeNoName(t *testing.T) {
	tm := blindFixedTime()
	e := logEntry{
		Time:       tm,
		Name:       "",
		Method:     "GET",
		URL:        "http://h/json",
		Status:     200,
		StatusText: "OK",
		Duration:   3 * time.Millisecond,
		Size:       0,
	}
	got := formatLogEntry(e)

	// Name == "" -> no bracket clause at all.
	if strings.Contains(got, "[") || strings.Contains(got, "]") {
		t.Errorf("formatLogEntry no-name: expected no %q/%q bracket in %q", "[", "]", got)
	}

	// Size <= 0 -> the size substring must be absent...
	sizeStr := formatSize(0)
	if strings.Contains(got, sizeStr) {
		t.Errorf("formatLogEntry no-size: expected size substring %q to be absent in %q", sizeStr, got)
	}
	// ...and there should be no trailing " · <size>" clause after the status.
	// The duration clause is still introduced by one " · "; the size clause is
	// the second one, so the no-size line must contain at most one " · ".
	if strings.Count(got, " · ") > 1 {
		t.Errorf("formatLogEntry no-size: expected at most one %q separator in %q", " · ", got)
	}

	// sanity: still a well-formed success line
	if !strings.Contains(got, "→ 200 OK") {
		t.Errorf("formatLogEntry no-size: expected %q in %q", "→ 200 OK", got)
	}
}

func TestFormatLogEntry_Error(t *testing.T) {
	tm := blindFixedTime()
	e := logEntry{
		Time:   tm,
		Name:   "Ping",
		Method: "POST",
		URL:    "http://h/x",
		Err:    "context deadline exceeded",
	}
	got := formatLogEntry(e)

	if !strings.Contains(got, "→ ERROR: context deadline exceeded") {
		t.Errorf("formatLogEntry error: expected %q in %q", "→ ERROR: context deadline exceeded", got)
	}
	if strings.Contains(got, "200") {
		t.Errorf("formatLogEntry error: unexpected %q in %q", "200", got)
	}
	// still carries the prefix fields
	for _, sub := range []string{"15:04:05", "[Ping]", "POST http://h/x"} {
		if !strings.Contains(got, sub) {
			t.Errorf("formatLogEntry error: expected substring %q in %q", sub, got)
		}
	}
}

func TestFormatLog_JoinAndEmpty(t *testing.T) {
	tm := blindFixedTime()
	e1 := logEntry{
		Time:       tm,
		Name:       "Ping",
		Method:     "GET",
		URL:        "http://h/json",
		Status:     200,
		StatusText: "OK",
		Duration:   3 * time.Millisecond,
		Size:       1234,
	}
	e2 := logEntry{
		Time:   tm,
		Name:   "Ping",
		Method: "POST",
		URL:    "http://h/x",
		Err:    "context deadline exceeded",
	}

	want := formatLogEntry(e1) + "\n" + formatLogEntry(e2)
	got := formatLog([]logEntry{e1, e2})
	if got != want {
		t.Errorf("formatLog join: got %q, want %q", got, want)
	}
	// exactly one newline, no trailing newline
	if n := strings.Count(got, "\n"); n != 1 {
		t.Errorf("formatLog join: expected exactly one newline, got %d in %q", n, got)
	}
	if strings.HasSuffix(got, "\n") {
		t.Errorf("formatLog join: unexpected trailing newline in %q", got)
	}

	// empty and nil slices -> ""
	if s := formatLog([]logEntry{}); s != "" {
		t.Errorf("formatLog empty: expected %q, got %q", "", s)
	}
	if s := formatLog(nil); s != "" {
		t.Errorf("formatLog nil: expected %q, got %q", "", s)
	}
}
