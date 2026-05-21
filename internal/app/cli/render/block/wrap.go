// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File wrap.go — block-local text wrap helper. spec-1.7 D-2 + R2 D7 M-3.
//
// **Why not width.Wrap?** render/width.Wrap is rune-boundary hard break
// (no word awareness). spec-1.7 WrapPolicy Soft is CJK-aware word break
// — prefer last ASCII space whose visual width fits, hard-break a single
// word longer than cols, never split wide rune / combining cluster.
//
// **R2 D2 删 inline ANSI scope** (❌-7): wrap ignores ANSI escape state
// for now. LLM output containing `\x1b[...m` bytes will be treated as
// raw chars and may corrupt cell grid in extreme cases. spec-1.7a
// future ANSI tokenizer.
//
// **R2 D6 None+Truncated 优先**: WrapPolicy=None truncates with "…" and
// signals caller (Message.Render) to skip Truncated marker double-add.

package block

import (
	"strings"
	"unicode/utf8"

	"github.com/sqlrush/opendbx/internal/app/cli/render/width"
)

// wrap applies ctx.Wrap policy to text and returns the wrapped lines
// (each ≤ cols visual width). Each '\n' in input starts a new wrapped
// segment. Empty input returns nil.
//
// Soft: word-aware (prefer ASCII space break); hard-break long words.
// Hard: rune-boundary break at cols (preserves wide-rune integrity).
// None: single-line truncate per input line; appends "…" on overflow.
func wrap(text string, cols int, policy WrapPolicy) []string {
	if text == "" {
		return nil
	}
	if cols <= 0 {
		return nil
	}
	rawLines := strings.Split(text, "\n")
	var out []string
	for _, line := range rawLines {
		switch policy {
		case WrapHard:
			out = append(out, wrapHard(line, cols)...)
		case WrapNone:
			out = append(out, wrapNone(line, cols))
		default: // WrapSoft
			out = append(out, wrapSoft(line, cols)...)
		}
	}
	return out
}

// wrapSoft is word-aware: scans left-to-right, breaks at the last ASCII
// space whose accumulated width fits cols; hard-breaks single words > cols.
// Wide runes (CJK) are kept whole (not split mid-cluster).
func wrapSoft(line string, cols int) []string {
	if line == "" {
		return []string{""}
	}
	if width.Width(line) <= cols {
		return []string{line}
	}
	var lines []string
	var cur strings.Builder
	curW := 0
	lastSpace := -1 // byte offset in cur where last ASCII space was emitted
	lastSpaceW := 0

	i := 0
	for i < len(line) {
		r, size := utf8.DecodeRuneInString(line[i:])
		rw := width.RuneWidth(r)
		if r == ' ' {
			// If adding the space overflows, flush current line.
			if curW+rw > cols {
				lines = append(lines, cur.String())
				cur.Reset()
				curW = 0
				lastSpace = -1
				lastSpaceW = 0
				i += size
				// Skip leading spaces on the new line for cleaner wrap.
				for i < len(line) && line[i] == ' ' {
					i++
				}
				continue
			}
			cur.WriteRune(r)
			curW += rw
			lastSpace = cur.Len()
			lastSpaceW = curW
			i += size
			continue
		}
		// Non-space rune.
		if curW+rw > cols {
			if lastSpace > 0 {
				// Break at last space: emit prefix, retain suffix.
				prefix := cur.String()[:lastSpace-1] // drop the trailing space
				suffix := cur.String()[lastSpace:]
				lines = append(lines, prefix)
				cur.Reset()
				cur.WriteString(suffix)
				curW = width.Width(suffix)
				lastSpace = -1
				lastSpaceW = 0
			} else {
				// No space to break at: hard-break the single word.
				lines = append(lines, cur.String())
				cur.Reset()
				curW = 0
			}
		}
		cur.WriteRune(r)
		curW += rw
		i += size
		_ = lastSpaceW
	}
	if cur.Len() > 0 || len(lines) == 0 {
		lines = append(lines, cur.String())
	}
	return lines
}

// wrapHard breaks at cols rune-boundary; preserves wide-rune integrity
// (never splits mid-wide-rune; pushes wide rune to next row instead).
func wrapHard(line string, cols int) []string {
	if line == "" {
		return []string{""}
	}
	if width.Width(line) <= cols {
		return []string{line}
	}
	var lines []string
	var cur strings.Builder
	curW := 0
	for i := 0; i < len(line); {
		r, size := utf8.DecodeRuneInString(line[i:])
		rw := width.RuneWidth(r)
		if curW+rw > cols {
			lines = append(lines, cur.String())
			cur.Reset()
			curW = 0
		}
		cur.WriteRune(r)
		curW += rw
		i += size
	}
	if cur.Len() > 0 {
		lines = append(lines, cur.String())
	}
	return lines
}

// wrapNone truncates a single line to cols-1 cells + "…" if overflow.
// R2 D6 priority: this "…" is internal to None policy and **suppresses**
// Message.Truncated marker double-add (Message.Render checks).
func wrapNone(line string, cols int) string {
	if line == "" {
		return ""
	}
	if width.Width(line) <= cols {
		return line
	}
	// Truncate to cols-1, append "…".
	var b strings.Builder
	w := 0
	for i := 0; i < len(line); {
		r, size := utf8.DecodeRuneInString(line[i:])
		rw := width.RuneWidth(r)
		if w+rw > cols-1 {
			break
		}
		b.WriteRune(r)
		w += rw
		i += size
	}
	b.WriteRune(markerRune) // "…"
	return b.String()
}
