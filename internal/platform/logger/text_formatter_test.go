// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package logger

import (
	"strings"
	"testing"
	"time"
)

func TestFormatEventBasic(t *testing.T) {
	t.Parallel()
	resetFormattedOutputForTest(t)
	ts := time.Date(2026, 5, 10, 14, 32, 1, 123_000_000, time.UTC)
	got := formatEvent(ts, LevelInfo, "hello world")
	want := "2026-05-10T14:32:01.123Z [INFO] hello world\n"
	if got != want {
		t.Errorf("formatEvent basic\n  got:  %q\n  want: %q", got, want)
	}
}

func TestFormatEventLevelUppercase(t *testing.T) {
	t.Parallel()
	resetFormattedOutputForTest(t)
	ts := time.Date(2026, 5, 10, 14, 32, 1, 123_000_000, time.UTC)
	cases := []struct {
		level Level
		want  string
	}{
		{LevelVerbose, "VERBOSE"},
		{LevelDebug, "DEBUG"},
		{LevelInfo, "INFO"},
		{LevelWarn, "WARN"},
		{LevelError, "ERROR"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()
			got := formatEvent(ts, tc.level, "x")
			if !strings.Contains(got, "["+tc.want+"]") {
				t.Errorf("formatEvent(%v) missing [%s]: %q", tc.level, tc.want, got)
			}
		})
	}
}

func TestFormatEventMessageTrim(t *testing.T) {
	t.Parallel()
	resetFormattedOutputForTest(t)
	ts := time.Date(2026, 5, 10, 14, 32, 1, 0, time.UTC)
	got := formatEvent(ts, LevelInfo, "   hello   ")
	want := "2026-05-10T14:32:01.000Z [INFO] hello\n"
	if got != want {
		t.Errorf("formatEvent trim\n  got:  %q\n  want: %q", got, want)
	}
}

func TestFormatEventEmptyMessage(t *testing.T) {
	t.Parallel()
	resetFormattedOutputForTest(t)
	ts := time.Date(2026, 5, 10, 14, 32, 1, 0, time.UTC)
	got := formatEvent(ts, LevelInfo, "")
	want := "2026-05-10T14:32:01.000Z [INFO] \n"
	if got != want {
		t.Errorf("formatEvent empty\n  got:  %q\n  want: %q", got, want)
	}
}

func TestFormatEventOnlyWhitespace(t *testing.T) {
	t.Parallel()
	resetFormattedOutputForTest(t)
	ts := time.Date(2026, 5, 10, 14, 32, 1, 0, time.UTC)
	got := formatEvent(ts, LevelInfo, "   \t  ")
	want := "2026-05-10T14:32:01.000Z [INFO] \n"
	if got != want {
		t.Errorf("formatEvent whitespace-only\n  got:  %q\n  want: %q", got, want)
	}
}

// TestFormatEventMultilinePreTUI: hasFormattedOutput=false (default, pre-TUI)
// passes multi-line through verbatim. CC parity contract: only the trailing
// trim is applied; embedded newlines stay (codex+claude HIGH-1).
func TestFormatEventMultilinePreTUI(t *testing.T) {
	t.Parallel()
	resetFormattedOutputForTest(t)
	// Explicitly assert pre-TUI default.
	if isFormattedOutput() {
		t.Fatal("hasFormattedOutput should be false in pre-TUI test")
	}
	ts := time.Date(2026, 5, 10, 14, 32, 1, 0, time.UTC)
	got := formatEvent(ts, LevelInfo, "line1\nline2")
	want := "2026-05-10T14:32:01.000Z [INFO] line1\nline2\n"
	if got != want {
		t.Errorf("formatEvent pre-TUI multiline\n  got:  %q\n  want: %q", got, want)
	}
}

// TestFormatEventMultilinePostTUI: hasFormattedOutput=true (post-TUI) wraps
// multi-line message via json.Marshal → single line.
func TestFormatEventMultilinePostTUI(t *testing.T) {
	resetFormattedOutputForTest(t)
	SetHasFormattedOutput(true)
	defer SetHasFormattedOutput(false)
	ts := time.Date(2026, 5, 10, 14, 32, 1, 0, time.UTC)
	got := formatEvent(ts, LevelInfo, "line1\nline2")
	// json.Marshal("line1\nline2") = `"line1\nline2"` (note quoted + escaped).
	want := "2026-05-10T14:32:01.000Z [INFO] \"line1\\nline2\"\n"
	if got != want {
		t.Errorf("formatEvent post-TUI multiline\n  got:  %q\n  want: %q", got, want)
	}
}

// TestFormatEventSingleLinePostTUI: hasFormattedOutput=true does NOT
// jsonStringify single-line messages. The guard checks for newline presence.
func TestFormatEventSingleLinePostTUI(t *testing.T) {
	resetFormattedOutputForTest(t)
	SetHasFormattedOutput(true)
	defer SetHasFormattedOutput(false)
	ts := time.Date(2026, 5, 10, 14, 32, 1, 0, time.UTC)
	got := formatEvent(ts, LevelInfo, "hello world")
	want := "2026-05-10T14:32:01.000Z [INFO] hello world\n"
	if got != want {
		t.Errorf("formatEvent post-TUI single-line\n  got:  %q\n  want: %q", got, want)
	}
}

// TestFormatEventANSIPreserved: ANSI escape sequences in messages survive
// formatting untouched. Important for CC parity (some debug messages embed
// colour codes; pre-TUI we keep them).
func TestFormatEventANSIPreserved(t *testing.T) {
	t.Parallel()
	resetFormattedOutputForTest(t)
	ts := time.Date(2026, 5, 10, 14, 32, 1, 0, time.UTC)
	msg := "\x1b[31mred\x1b[0m"
	got := formatEvent(ts, LevelInfo, msg)
	if !strings.Contains(got, msg) {
		t.Errorf("ANSI not preserved\n  got: %q\n  want substring: %q", got, msg)
	}
}

// TestFormatEventUnicode: Chinese / emoji / non-ASCII characters survive.
func TestFormatEventUnicode(t *testing.T) {
	t.Parallel()
	resetFormattedOutputForTest(t)
	ts := time.Date(2026, 5, 10, 14, 32, 1, 0, time.UTC)
	cases := []string{
		"中文测试",
		"emoji: 🚀✅❌",
		"mixed 中英 🚀 文本",
	}
	for _, msg := range cases {
		got := formatEvent(ts, LevelInfo, msg)
		if !strings.Contains(got, msg) {
			t.Errorf("unicode not preserved\n  got: %q\n  want substring: %q", got, msg)
		}
	}
}

// TestFormatEventCategoryPattern: messages that look like `[CATEGORY] msg`
// (a CC filter pattern) format correctly without confusing the formatter.
// Note: the formatter does NOT do filter parsing — that's D-3's job (filter.go).
func TestFormatEventCategoryPattern(t *testing.T) {
	t.Parallel()
	resetFormattedOutputForTest(t)
	ts := time.Date(2026, 5, 10, 14, 32, 1, 0, time.UTC)
	got := formatEvent(ts, LevelInfo, "[MCP] server starting")
	want := "2026-05-10T14:32:01.000Z [INFO] [MCP] server starting\n"
	if got != want {
		t.Errorf("formatEvent category pattern\n  got:  %q\n  want: %q", got, want)
	}
}

func TestFormatTimestampMillisecondPrecision(t *testing.T) {
	t.Parallel()
	// 123 ms → ".123Z"; 0 ms → ".000Z"; CC parity check.
	cases := []struct {
		ns   int
		want string
	}{
		{0, "2026-05-10T14:32:01.000Z"},
		{1_000_000, "2026-05-10T14:32:01.001Z"},
		{123_000_000, "2026-05-10T14:32:01.123Z"},
		{999_000_000, "2026-05-10T14:32:01.999Z"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()
			ts := time.Date(2026, 5, 10, 14, 32, 1, tc.ns, time.UTC)
			got := formatTimestamp(ts)
			if got != tc.want {
				t.Errorf("formatTimestamp(%d ns) = %q, want %q", tc.ns, got, tc.want)
			}
		})
	}
}

// TestFormatTimestampUTC: even when input has a non-UTC timezone, output is
// converted to UTC (CC's toISOString always emits Z).
func TestFormatTimestampUTC(t *testing.T) {
	t.Parallel()
	// 14:32:01 +08:00 → 06:32:01 UTC.
	loc := time.FixedZone("CST", 8*3600)
	ts := time.Date(2026, 5, 10, 14, 32, 1, 0, loc)
	got := formatTimestamp(ts)
	want := "2026-05-10T06:32:01.000Z"
	if got != want {
		t.Errorf("formatTimestamp non-UTC\n  got:  %q\n  want: %q", got, want)
	}
}

// resetFormattedOutputForTest resets the package-level hasFormattedOutput
// flag and registers a cleanup so parallel tests don't leak state.
func resetFormattedOutputForTest(t *testing.T) {
	t.Helper()
	prev := isFormattedOutput()
	SetHasFormattedOutput(false)
	t.Cleanup(func() { SetHasFormattedOutput(prev) })
}
