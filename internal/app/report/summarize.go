// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File summarize.go — bound a tool result for the report body (spec-1.23 D-2).
//
// A tool result can be large (a wide pg_stat dump). The report truncates it to
// keep the markdown readable. Truncation is UTF-8 / CJK safe: it never splits a
// multi-byte rune (which would render as mojibake and fail the spec-1.20.2
// no-mojibake gate). It bounds by BOTH line count and byte size, cutting at the
// last whole line within the byte ceiling.

package report

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	// maxResultLines caps how many lines of a single tool result appear.
	maxResultLines = 40
	// maxResultBytes caps the byte size; whichever limit is hit first wins.
	maxResultBytes = 4096
)

// summarize truncates content to at most maxResultLines lines and
// maxResultBytes bytes, cutting at line boundaries (and, for a single
// over-cap line, at a rune boundary). When truncated it appends
// "… (truncated N more lines)" where N is the number of dropped lines.
func summarize(content string) string {
	if content == "" {
		return ""
	}
	lines := strings.Split(content, "\n")
	total := len(lines)

	// Line bound first.
	kept := lines
	if total > maxResultLines {
		kept = lines[:maxResultLines]
	}

	// Byte bound: keep the largest prefix of whole lines within the ceiling.
	keptCount := lineFitInBytes(kept, maxResultBytes)
	if keptCount == 0 {
		// Even the first line exceeds the byte ceiling — truncate that single
		// line on a rune boundary so we never emit a partial UTF-8 sequence.
		// This is a byte-cut of one line, not a line drop, so the marker omits
		// the line count.
		head := cutRunes(kept[0], maxResultBytes)
		return head + "\n… (truncated)"
	}

	out := strings.Join(kept[:keptCount], "\n")
	dropped := total - keptCount
	if dropped <= 0 {
		return out
	}
	return appendMarker(out, dropped)
}

// lineFitInBytes returns how many leading lines (joined by "\n") fit within
// max bytes. Always returns at least 0.
func lineFitInBytes(lines []string, max int) int {
	used := 0
	for i, ln := range lines {
		add := len(ln)
		if i > 0 {
			add++ // the joining newline
		}
		if used+add > max {
			return i
		}
		used += add
	}
	return len(lines)
}

// cutRunes returns the longest prefix of s whose byte length is <= max,
// without splitting a multi-byte rune.
func cutRunes(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max
	// Back up to a rune boundary (utf8.RuneStart marks a leading byte).
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

func appendMarker(s string, droppedLines int) string {
	return s + fmt.Sprintf("\n… (truncated %d more lines)", droppedLines)
}
