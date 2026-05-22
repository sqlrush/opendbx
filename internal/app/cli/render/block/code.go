// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package block

import (
	"strings"

	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
	"github.com/sqlrush/opendbx/internal/app/cli/render/style"
	"github.com/sqlrush/opendbx/internal/app/cli/render/width"
)

// Code is the spec-0.13 stub for the code block type. Render returns
// (nil, ErrUnsupportedNode) per the spec-0.13 D-3 contract.
//
// **spec-1.7 R2 D6 HIGH-C double duty**: code.go hosts (a) the Code stub
// type with Render returning ErrUnsupportedNode (spec-0.13 contract
// preserved per R2 D6); (b) private helper `renderCodeBlock` used by
// block.Message.Render fence path (spec-1.7 D-4).
//
// TODO(spec-1.12): real Code.Render impl in spec-1.12 code-highlight-block.
type Code struct{}

// Render satisfies the RenderNode interface but returns the unsupported
// sentinel for this stub. spec-1.12 code-highlight-block will replace.
func (Code) Render(_ Context) (buffer.Buffer, error) {
	// errcode-lint:exempt -- spec-0.13 D-3: ErrUnsupportedNode is the registered sentinel; spec-1.12 code-highlight-block replaces with real Render.
	return nil, ErrUnsupportedNode
}

// renderCodeBlock is the private helper used by block.Message.Render
// fence path (spec-1.7 D-4). Renders a fenced code segment with:
//   - lang label first row: `─── <lang> ───` (or `───────` if lang empty)
//     using StyleLangLabel from ctx.Theme.
//   - 1-cell horizontal padding (left + right).
//   - Code body rows: StyleCode foreground + StyleCodeBg background
//     across the entire cell row width (full-width bg per CC visual).
//   - MeasureOnly path returns measureOnlyBuf with correct rows count.
//
// Returns the rendered Buffer and the row count.
//
// **R2 D6 HIGH-D**: markers (Truncated/Continued) are NOT applied here.
// renderMixed applies markers to the stitched buffer AFTER concat.
func renderCodeBlock(ctx Context, lang, body string) (buffer.Buffer, int) {
	theme := themeOrDefault(ctx.Theme)
	bodyLines := strings.Split(body, "\n")
	// Drop trailing empty line from Split if body ends with newline.
	if len(bodyLines) > 0 && bodyLines[len(bodyLines)-1] == "" {
		bodyLines = bodyLines[:len(bodyLines)-1]
	}
	rows := 1 + len(bodyLines) // 1 lang label row + body rows
	if ctx.MeasureOnly {
		return measureOnlyBuf(ctx.Cols, rows), rows
	}
	buf, err := buffer.NewGrid(ctx.Cols, rows)
	if err != nil {
		return measureOnlyBuf(ctx.Cols, rows), rows
	}
	// Row 0: lang label `─── <lang> ───` (placeholder visual per R2 D5
	// fixture lock-in; T-9 R3 may errata).
	labelStyle := theme.Style(StyleLangLabel)
	label := buildLangLabel(lang, ctx.Cols)
	for x, r := range []rune(label) {
		if x >= ctx.Cols {
			break
		}
		buf.SetCell(x, 0, buffer.Cell{Ch: r, St: labelStyle})
	}
	// Body rows: full-width StyleCodeBg + StyleCode foreground.
	bgStyle := theme.Style(StyleCodeBg)
	codeStyle := theme.Style(StyleCode)
	mergedStyle := mergeStyles(codeStyle, bgStyle)
	for i, line := range bodyLines {
		y := 1 + i
		// Fill full row with background first.
		for x := 0; x < ctx.Cols; x++ {
			buf.SetCell(x, y, buffer.Cell{Ch: ' ', St: mergedStyle})
		}
		// Overlay code chars starting at x=1 (1-cell left padding).
		// spec-1.7 T-9 HIGH-2: advance x by RuneWidth so wide runes (CJK)
		// occupy 2 cells; SetCell auto-writes WideContinuation at x+1 and
		// x++ would clobber via clearWideOverlap invariant.
		x := 1
		for _, r := range line {
			rw := width.RuneWidth(r)
			if rw <= 0 {
				continue
			}
			if x+rw > ctx.Cols-1 { // 1-cell right padding
				break
			}
			buf.SetCell(x, y, buffer.Cell{Ch: r, St: mergedStyle})
			x += rw
		}
	}
	return buf, rows
}

// buildLangLabel returns `─── <lang> ───` centered-ish for a given
// label and cols width. If lang is empty, returns all-dash row.
func buildLangLabel(lang string, cols int) string {
	if cols <= 0 {
		return ""
	}
	if lang == "" {
		return strings.Repeat("─", cols)
	}
	mid := " " + lang + " "
	dashTotal := cols - len([]rune(mid))
	if dashTotal < 6 {
		// Cols too narrow for centered label; just left-anchor.
		return strings.Repeat("─", 3) + mid
	}
	left := dashTotal / 2
	right := dashTotal - left
	return strings.Repeat("─", left) + mid + strings.Repeat("─", right)
}

// mergeStyles overlays fg with bg attributes (bg takes BG only, fg keeps FG).
func mergeStyles(fg, bg style.Style) style.Style {
	out := fg
	if bg.BG != 0 {
		out.BG = bg.BG
	}
	return out
}
