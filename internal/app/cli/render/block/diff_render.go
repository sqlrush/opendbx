// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File diff_render.go — spec-1.13 D-3 per-line body cell builder.
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

package block

import (
	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
)

// renderDiffHunkBody returns the body cells (post-marker, post-gutter)
// for one LineEntry. Chroma highlight applies only when BodyLang is
// non-empty AND theme is HighlighterTheme AND marker is NOT '-' (R3
// CRIT-2 ★A CC parity skip).
//
// Returns plain rune-cells with empty style.Style for plain path;
// chroma-styled cells for highlighted path. Caller (diff.go
// writeDiffRow) overlays nothing (CRIT-1 ★A — marker color is on
// prefix column only).
func renderDiffHunkBody(marker rune, body, lang string, ctx Context, theme StyleTheme, hl HighlighterTheme) []buffer.Cell {
	// R3 CRIT-2 ★A: `-` removed line → skip chroma highlight per CC
	// color-diff/index.ts:915-918 (defaultStyle plain text).
	if marker == '-' || lang == "" || hl == nil {
		return plainBodyCells(body)
	}
	rows, err := highlight(lang, body, hl, ctx.ColorDepth)
	if err != nil || len(rows) == 0 {
		return plainBodyCells(body)
	}
	// body is a single line; highlight returns one highlightedRow.
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
