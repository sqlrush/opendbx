// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package report

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSummarize_UnderLimits_Unchanged(t *testing.T) {
	t.Parallel()
	in := "line1\nline2\nline3"
	if got := summarize(in); got != in {
		t.Errorf("under-limit content changed: %q", got)
	}
}

func TestCutRunes(t *testing.T) {
	t.Parallel()
	if got := cutRunes("abc", 10); got != "abc" {
		t.Errorf("under-max: %q", got) // len <= max → unchanged
	}
	// "中" is 3 bytes; max=4 lands inside the 2nd rune → back up to 3.
	if got := cutRunes("中中中", 4); got != "中" {
		t.Errorf("mid-rune cut = %q; want 中", got)
	}
	if got := cutRunes("中中中", 6); got != "中中" {
		t.Errorf("rune-boundary cut = %q; want 中中", got)
	}
}

func TestSummarize_Empty(t *testing.T) {
	t.Parallel()
	if got := summarize(""); got != "" {
		t.Errorf("empty → %q", got)
	}
}

func TestSummarize_OverLines_Truncated(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	for i := 0; i < 50; i++ {
		b.WriteString("x\n")
	}
	got := summarize(strings.TrimRight(b.String(), "\n")) // 50 lines
	if strings.Count(got, "\n… (truncated") != 1 {
		t.Fatalf("expected one truncation marker: %q", got)
	}
	if !strings.Contains(got, "(truncated 10 more lines)") {
		t.Errorf("expected 10 dropped lines (50-40): %q", got)
	}
	// kept body is exactly maxResultLines lines (the marker is an extra line).
	body := strings.SplitN(got, "\n… (truncated", 2)[0]
	if n := strings.Count(body, "\n") + 1; n != maxResultLines {
		t.Errorf("kept %d lines; want %d", n, maxResultLines)
	}
}

func TestSummarize_OverBytes_CutsWholeLines(t *testing.T) {
	t.Parallel()
	// 10 lines, each 1000 bytes → ~10KB, well over 4096; only whole lines kept.
	line := strings.Repeat("a", 1000)
	lines := make([]string, 10)
	for i := range lines {
		lines[i] = line
	}
	got := summarize(strings.Join(lines, "\n"))
	if len(got) > maxResultBytes+64 { // body within ceiling (+marker slack)
		t.Errorf("over-bytes not bounded: %d bytes", len(got))
	}
	if !strings.Contains(got, "more lines)") {
		t.Errorf("expected line-drop marker: %q", got[:80])
	}
	// every kept line is whole (no partial 'a' run mid-line cut).
	body := strings.SplitN(got, "\n… (truncated", 2)[0]
	for _, ln := range strings.Split(body, "\n") {
		if ln != line {
			t.Fatalf("kept line not whole: %d bytes", len(ln))
		}
	}
}

// TestSummarize_SingleHugeLine_RuneSafe — a single line larger than the byte
// ceiling is cut on a rune boundary (never mid-UTF-8), so no mojibake.
func TestSummarize_SingleHugeLine_RuneSafe(t *testing.T) {
	t.Parallel()
	// 2000 CJK runes × 3 bytes = 6000 bytes, one line, > 4096.
	huge := strings.Repeat("中", 2000)
	got := summarize(huge)
	if !utf8.ValidString(got) {
		t.Fatal("summarize produced invalid UTF-8 (split a multi-byte rune)")
	}
	body := strings.TrimSuffix(got, "\n… (truncated)")
	if body == got {
		t.Errorf("expected byte-cut marker: %q", got[len(got)-40:])
	}
	if len(body) > maxResultBytes {
		t.Errorf("byte-cut body %d > ceiling %d", len(body), maxResultBytes)
	}
	// body is a whole number of '中' runes.
	if strings.Count(body, "中")*3 != len(body) {
		t.Errorf("body not a whole number of runes: %d bytes", len(body))
	}
}

// TestSummarize_ByteCapMidCJK_Boundary — the byte cap landing inside a CJK rune
// must back up to the rune start, not split it.
func TestSummarize_ByteCapMidCJK_Boundary(t *testing.T) {
	t.Parallel()
	// Construct a single line whose length crosses maxResultBytes mid-rune:
	// 1366 '中' = 4098 bytes (> 4096); 4096 is inside the 1366th rune.
	line := strings.Repeat("中", 1366)
	got := summarize(line)
	if !utf8.ValidString(got) {
		t.Fatal("byte-cap mid-CJK produced invalid UTF-8")
	}
}
