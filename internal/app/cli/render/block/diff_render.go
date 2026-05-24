// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File diff_render.go — spec-1.13 D-3 body cell builder + R2 HIGH-3
// per-hunk batch chroma highlight.
//
// Per R3 CRIT-2 ★A (B-38 CC parity):
//   - `-` removed lines skip chroma highlight; body uses StyleNormal
//     (defaultStyle equivalent) — 让用户聚焦"什么变了"不是旧代码语法
//   - `+`/' '/'?' lines do chroma highlight via spec-1.12 highlight()
//     when BodyLang != "" AND theme implements HighlighterTheme
//   - Highlight failure / unknown lang → plain cells (I-2 contract)
//
// Per R2 CRIT-1 ★A: NO row-wide extraAttrs.FG overlay on body cells —
// marker color lives on prefix column (diff.go writeDiffRow); body
// cells keep chroma token colors.
//
// Per R2 HIGH-3: chroma is invoked ONCE per hunk on the joined non-'-'
// body (preserves multi-line lexer context for block comments / string
// literals); single-line per-call fallback path kept for the auto-detect
// NewDiff fallback and unit-test convenience.

package block

import (
	"strings"

	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
)

// hunkBodyCells holds pre-rendered body cells for one hunk, keyed by the
// LineEntry index within Hunk.Lines. `-` marker lines are absent from the
// map (callers fall back to plainBodyCells). nil result means batch
// highlight is unavailable (no lang / no highlighter / chroma failure)
// and every line must use plainBodyCells.
type hunkBodyCells map[int][]buffer.Cell

// highlightHunkBatch runs a single chroma pass over the joined non-'-'
// body lines of the hunk, preserving multi-line lexer context (block
// comments / multi-line string literals would otherwise be misclassified
// at line boundaries — spec-1.13 R2 HIGH-3 correctness fix).
//
// Returns nil when batch highlight is unavailable; caller falls back to
// plainBodyCells per LineEntry. Returns nil on any row-count mismatch
// (defensive — chroma should yield exactly one row per joined `\n`
// segment, but a multi-line token would compress two segments into one).
func highlightHunkBatch(h Hunk, lang string, depth int, hl HighlighterTheme) hunkBodyCells {
	if lang == "" || hl == nil {
		return nil
	}

	indices := make([]int, 0, len(h.Lines))
	for i, ln := range h.Lines {
		if ln.Marker == '-' {
			continue
		}
		indices = append(indices, i)
	}
	if len(indices) == 0 {
		return nil
	}

	var b strings.Builder
	for i, idx := range indices {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(h.Lines[idx].Text)
	}

	rows, err := highlight(lang, b.String(), hl, depth)
	if err != nil || len(rows) != len(indices) {
		return nil
	}

	out := make(hunkBodyCells, len(indices))
	for i, idx := range indices {
		out[idx] = rows[i].cells
	}
	return out
}

// renderDiffHunkBody returns the body cells (post-marker, post-gutter)
// for one LineEntry — single-line fallback path. Used by:
//
//  1. R2 HIGH-3 batch path when the hunk index is not in the prepared
//     hunkBodyCells map (fallback when batch returned nil).
//  2. Unit tests that exercise the per-line code path directly.
//
// Chroma highlight applies only when BodyLang is non-empty AND theme is
// HighlighterTheme AND marker is NOT '-' (R3 CRIT-2 ★A CC parity skip).
//
// Returns plain rune-cells with empty style.Style for plain path;
// chroma-styled cells for highlighted path. Caller (diff.go
// writeDiffRow) overlays nothing (CRIT-1 ★A — marker color is on
// prefix column only).
func renderDiffHunkBody(marker rune, body, lang string, ctx Context, theme StyleTheme, hl HighlighterTheme) []buffer.Cell {
	_ = theme
	// R3 CRIT-2 ★A: `-` removed line → skip chroma highlight per CC
	// color-diff/index.ts:915-918 (defaultStyle plain text).
	if marker == '-' || lang == "" || hl == nil {
		return plainBodyCells(body)
	}
	rows, err := highlight(lang, body, hl, ctx.ColorDepth)
	if err != nil || len(rows) == 0 {
		return plainBodyCells(body)
	}
	return rows[0].cells
}

// plainBodyCells returns rune-cells with empty style.Style (StyleNormal).
// Caller's writeDiffRow may substitute Style{} → theme.Style(StyleNormal)
// at paint time.
func plainBodyCells(body string) []buffer.Cell {
	out := make([]buffer.Cell, 0, len(body))
	for _, r := range body {
		out = append(out, buffer.Cell{Ch: r})
	}
	return out
}
