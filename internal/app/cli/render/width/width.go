// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package width

import (
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

// condition pins EastAsianWidth=false (CLAUDE.md § 3.1 invariant) per
// the same pattern as spec-0.11.5 uiinvariant.widthCondition.
//
//nolint:gochecknoglobals // spec-0.13 D-2: package-level condition is the single source of truth.
var condition = &runewidth.Condition{
	EastAsianWidth:     false,
	StrictEmojiNeutral: true,
}

// ErrInvalidWidth is returned (via panic) when Wrap/Truncate/Pad receive
// non-positive dimensions. System-boundary fail-fast per CLAUDE coding
// style; callers must validate dimensions from terminal resize events.
//
//nolint:gochecknoglobals // spec-0.6 contract: errcode sentinels are package-level.
var ErrInvalidWidth = errcode.Register(
	"RENDER.INVALID_WIDTH",
	"width function called with non-positive dimension",
	"validate cols/max/target > 0 from terminal resize event before calling width.Wrap/Truncate/Pad",
)

// Width returns the visual column width of s under EastAsianWidth=false.
// Combining marks and ZWJ sequences collapse per runewidth semantics.
// Returns 0 for empty input.
func Width(s string) int {
	return condition.StringWidth(s)
}

// RuneWidth returns the visual column width of a single rune r under
// EastAsianWidth=false. ASCII fast path (r ≤ 0x7F → 1) avoids string
// allocation + runewidth scan for the dominant common case; non-ASCII
// runes delegate to Width(string(r)) using the package condition.
//
// Practical return values: 0 (combining marks / zero-width), 1 (narrow,
// ASCII / Latin), or 2 (East Asian wide / CJK). Callers should treat
// values other than 2 as narrow (1-cell main write, no continuation).
//
// Per-rune SSOT: spec-0.13 § 11.7 errata adds this fn so callers do
// not scatter ASCII branches. buffer.SetCell uses RuneWidth(c.Ch) to
// auto-detect wide rune continuation.
func RuneWidth(r rune) int {
	if r >= 0 && r <= 0x7F {
		return 1
	}
	return condition.RuneWidth(r)
}

// Wrap breaks s into lines, each visually ≤ cols columns. Lines are split
// on rune boundaries; no hyphenation or word-aware wrapping (callers
// needing CJK-aware word breaks should pre-process with a higher-level
// tokenizer). Returns []string{} for empty input; never returns nil.
//
// Panics with ErrInvalidWidth if cols ≤ 0.
//
// Degenerate behavior (spec-0.13 T-13 security-reviewer R1 LOW-1 / code-reviewer
// MED-1 pin): when a single rune is wider than cols (e.g., CJK rune with
// cols=1), Wrap emits an empty line before the rune and continues. This
// is the current spec-0.13 D-2 contract; spec-1.3 may revisit (drop rune
// or substitute ellipsis).
//
// Input-size bound (security-reviewer MED-2): Caller is responsible for
// bounding input size before calling Wrap. spec-3.10 context-budget
// enforces the upstream bound at the LLM streaming boundary.
func Wrap(s string, cols int) []string {
	if cols <= 0 {
		panic(ErrInvalidWidth)
	}
	if s == "" {
		return []string{}
	}
	var lines []string
	var cur strings.Builder
	curWidth := 0
	for _, r := range s {
		rw := condition.RuneWidth(r)
		if curWidth+rw > cols {
			lines = append(lines, cur.String())
			cur.Reset()
			curWidth = 0
		}
		cur.WriteRune(r)
		curWidth += rw
	}
	if cur.Len() > 0 || len(lines) == 0 {
		lines = append(lines, cur.String())
	}
	return lines
}

// Truncate cuts s to ≤ max visual columns, appending "…" (one column)
// if any rune was removed. If s already fits, returns s unchanged.
//
// Panics with ErrInvalidWidth if max ≤ 0.
func Truncate(s string, max int) string {
	if max <= 0 {
		panic(ErrInvalidWidth)
	}
	if condition.StringWidth(s) <= max {
		return s
	}
	// Reserve 1 column for the ellipsis.
	budget := max - 1
	if budget <= 0 {
		return "…"
	}
	var b strings.Builder
	curWidth := 0
	for _, r := range s {
		rw := condition.RuneWidth(r)
		if curWidth+rw > budget {
			break
		}
		b.WriteRune(r)
		curWidth += rw
	}
	b.WriteRune('…')
	return b.String()
}

// Pad right-pads s with ASCII spaces so the visual width equals target.
// If s already wider than target, returns s unchanged (caller responsibility
// to Truncate first if width must not exceed).
//
// Panics with ErrInvalidWidth if target ≤ 0.
func Pad(s string, target int) string {
	if target <= 0 {
		panic(ErrInvalidWidth)
	}
	cur := condition.StringWidth(s)
	if cur >= target {
		return s
	}
	return s + strings.Repeat(" ", target-cur)
}
