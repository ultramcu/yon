package ui

import (
	"testing"
	"time"
)

// fixedLogTime is a deterministic timestamp so the formatter's "15:04:05" clause
// is stable across runs (15:04:05 UTC).
var fixedLogTime = time.Date(2026, 6, 5, 15, 4, 5, 0, time.UTC)

// TestFormatLogEntry_Success_WithNameAndSize proves the full success line:
// timestamp, [Name], method, URL, status + text, duration, and size.
func TestFormatLogEntry_Success_WithNameAndSize_Dev(t *testing.T) {
	e := logEntry{
		Time:       fixedLogTime,
		Name:       "Get user",
		Method:     "GET",
		URL:        "https://api.example.com/users/1",
		Status:     200,
		StatusText: "OK",
		Duration:   123 * time.Millisecond,
		Size:       2048,
	}
	got := formatLogEntry(e)
	want := "15:04:05  [Get user]  GET https://api.example.com/users/1 → 200 OK · 123 ms · 2.0 KB"
	if got != want {
		t.Fatalf("formatLogEntry success with name+size:\n got: %q\nwant: %q", got, want)
	}
}

// TestFormatLogEntry_Success_NoName omits the "[...]" clause when Name is empty.
func TestFormatLogEntry_Success_NoName_Dev(t *testing.T) {
	e := logEntry{
		Time:       fixedLogTime,
		Method:     "POST",
		URL:        "https://api.example.com/login",
		Status:     201,
		StatusText: "Created",
		Duration:   50 * time.Millisecond,
		Size:       512,
	}
	got := formatLogEntry(e)
	want := "15:04:05  POST https://api.example.com/login → 201 Created · 50 ms · 512 B"
	if got != want {
		t.Fatalf("formatLogEntry success without name:\n got: %q\nwant: %q", got, want)
	}
}

// TestFormatLogEntry_Success_NoSize omits the "· <size>" clause when Size <= 0.
func TestFormatLogEntry_Success_NoSize_Dev(t *testing.T) {
	e := logEntry{
		Time:       fixedLogTime,
		Name:       "Ping",
		Method:     "HEAD",
		URL:        "https://api.example.com/ping",
		Status:     204,
		StatusText: "No Content",
		Duration:   2 * time.Second,
		Size:       0,
	}
	got := formatLogEntry(e)
	want := "15:04:05  [Ping]  HEAD https://api.example.com/ping → 204 No Content · 2.00 s"
	if got != want {
		t.Fatalf("formatLogEntry success without size:\n got: %q\nwant: %q", got, want)
	}
}

// TestFormatLogEntry_Error renders the ERROR line and drops status/size.
func TestFormatLogEntry_Error_Dev(t *testing.T) {
	e := logEntry{
		Time:   fixedLogTime,
		Name:   "Broken",
		Method: "GET",
		URL:    "https://nope.invalid/",
		Err:    "dial tcp: lookup nope.invalid: no such host",
	}
	got := formatLogEntry(e)
	want := "15:04:05  [Broken]  GET https://nope.invalid/ → ERROR: dial tcp: lookup nope.invalid: no such host"
	if got != want {
		t.Fatalf("formatLogEntry error:\n got: %q\nwant: %q", got, want)
	}
}

// TestFormatLogEntry_Error_NoName proves the error line also omits an empty name.
func TestFormatLogEntry_ErrorNoName_Dev(t *testing.T) {
	e := logEntry{
		Time:   fixedLogTime,
		Method: "DELETE",
		URL:    "https://api.example.com/x",
		Err:    "context deadline exceeded",
	}
	got := formatLogEntry(e)
	want := "15:04:05  DELETE https://api.example.com/x → ERROR: context deadline exceeded"
	if got != want {
		t.Fatalf("formatLogEntry error without name:\n got: %q\nwant: %q", got, want)
	}
}

// TestFormatLog_Join proves entries are joined by "\n" with no trailing newline.
func TestFormatLog_Join_Dev(t *testing.T) {
	entries := []logEntry{
		{Time: fixedLogTime, Name: "A", Method: "GET", URL: "u1", Status: 200, StatusText: "OK", Duration: time.Millisecond},
		{Time: fixedLogTime, Method: "GET", URL: "u2", Err: "boom"},
	}
	got := formatLog(entries)
	want := formatLogEntry(entries[0]) + "\n" + formatLogEntry(entries[1])
	if got != want {
		t.Fatalf("formatLog join:\n got: %q\nwant: %q", got, want)
	}
	if want == "" {
		t.Fatal("test setup: want should be non-empty")
	}
}

// TestFormatLog_Empty proves an empty/nil slice yields "".
func TestFormatLog_Empty_Dev(t *testing.T) {
	if got := formatLog(nil); got != "" {
		t.Fatalf("formatLog(nil) = %q, want \"\"", got)
	}
	if got := formatLog([]logEntry{}); got != "" {
		t.Fatalf("formatLog(empty) = %q, want \"\"", got)
	}
}
