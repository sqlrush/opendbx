// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File welcome.go — production Welcome panel block (spec-1.25 D-1 + errata R3).
// Renames + replaces the spec-0.13 Banner stub (grep-confirmed zero
// production callers). Renders an ASCII-art logo welcome whose STRUCTURE
// mirrors CC's WelcomeV2.tsx (logo-left + welcome text-right + cwd + tip);
// the logo GLYPHS are an opendbx-original block-drawing DB mark, NOT Claude's
// brand logo (CLAUDE.md § 3.2: do not copy CC brand assets). `✻` = CC
// figures.ts:6 TEARDROP_ASTERISK. render-only: never enters the llm wire.
// errata R3 (用户 Layer-5 拍板 2026-06-02): replaced the R2 boxed
// `╭─ ✻ Welcome ─╮` approximation with this logo form.

package block

import (
	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
	"github.com/sqlrush/opendbx/internal/app/cli/render/style"
	"github.com/sqlrush/opendbx/internal/app/cli/render/width"
)

// welcomeTitle is the welcome headline (CC parity "Welcome to Claude Code!"
// → opendbx). `✻` = figures.ts:6 TEARDROP_ASTERISK.
const welcomeTitle = "✻ Welcome to opendbx!"

// welcomeMark is the opendbx-original 3-row block-drawing DB mark rendered
// at the left of the welcome (an opendbx asset, not Claude's logo). All
// glyphs are width-1 box/block-drawing runes (EastAsianWidth=false).
var welcomeMark = [3]string{
	"▛▀▀▀▀▜",
	"▌▒▒▒▒▐",
	"▙▄▄▄▄▟",
}

const (
	welcomeMarkW = 6 // visual width of each welcomeMark row
	welcomeGap   = 2 // spaces between the mark and the text column
)

// welcomeTextCol is the column where the right-side text begins.
const welcomeTextCol = welcomeMarkW + welcomeGap

// Welcome is the spec-1.25 D-1 production welcome panel (replaces the
// spec-0.13 Banner stub). Zero-value Welcome{} renders the logo with no
// version/cwd/tip (it does NOT return ErrUnsupportedNode — that was the
// stub contract).
type Welcome struct {
	Version string // build version (internal/platform/version.String())
	Cwd     string // working dir, already ~-abbreviated by the caller
	Tip     string // single static onboarding tip (non-random; replayable)
}

// NewWelcome constructs a Welcome panel. Empty fields are simply omitted
// (e.g. version="" drops the version suffix; tip="" drops the tip line).
func NewWelcome(version, cwd, tip string) Welcome {
	return Welcome{Version: version, Cwd: cwd, Tip: tip}
}

// Render produces the logo welcome Buffer: 3 logo rows paired with text
// (headline / version / cwd) + a blank + the tip line. Graceful: when Cols
// is too small for the logo+text it falls back to a single line
// `✻ Welcome to opendbx <version>` (spec-1.25 D-1 / R-2). MeasureOnly
// returns the correct row count with no cell writes (spec-1.7 contract).
func (w Welcome) Render(ctx Context) (buffer.Buffer, error) {
	if ctx.Cols <= 0 {
		return measureOnlyBuf(0, 0), nil
	}
	verLine := "opendbx"
	if w.Version != "" {
		verLine += " " + w.Version
	}
	cwdLine := ""
	if w.Cwd != "" {
		cwdLine = "cwd: " + w.Cwd
	}
	widest := max(width.Width(verLine), width.Width(welcomeTitle), width.Width(cwdLine))
	if ctx.Cols < welcomeTextCol+widest {
		return w.renderFallback(ctx), nil
	}

	rows := len(welcomeMark) // 3 logo rows
	if w.Tip != "" {
		rows += 2 // blank spacer + tip line
	}
	if ctx.MeasureOnly {
		return measureOnlyBuf(ctx.Cols, rows), nil
	}
	buf, err := buffer.NewGrid(ctx.Cols, rows)
	if err != nil {
		return measureOnlyBuf(ctx.Cols, rows), nil
	}
	theme := themeOrDefault(ctx.Theme)
	dim := theme.Style(StyleDimmed)
	normal := theme.Style(StyleNormal)

	// Logo art at column 0 (accent/normal).
	for i, art := range welcomeMark {
		writeRunes(buf, 0, i, art, normal, welcomeMarkW)
	}
	// Row 0: "opendbx" (normal) + " <version>" (dim).
	x := writeRunes(buf, welcomeTextCol, 0, "opendbx", normal, ctx.Cols)
	if w.Version != "" {
		writeRunes(buf, x, 0, " "+w.Version, dim, ctx.Cols)
	}
	// Row 1: "✻ Welcome to opendbx!" (normal).
	writeRunes(buf, welcomeTextCol, 1, welcomeTitle, normal, ctx.Cols)
	// Row 2: "cwd: <cwd>" (dim).
	if cwdLine != "" {
		writeRunes(buf, welcomeTextCol, 2, cwdLine, dim, ctx.Cols)
	}
	// Row 4 (after a blank row 3): tip (dim), truncated at Cols.
	if w.Tip != "" {
		writeRunes(buf, welcomeTextCol, 4, w.Tip, dim, ctx.Cols)
	}
	return buf, nil
}

// renderFallback returns a single dim line "✻ Welcome to opendbx <version>"
// truncated to Cols, used when the terminal is too narrow for the logo.
func (w Welcome) renderFallback(ctx Context) buffer.Buffer {
	if ctx.MeasureOnly {
		return measureOnlyBuf(ctx.Cols, 1)
	}
	line := welcomeTitle
	if w.Version != "" {
		line += " " + w.Version
	}
	buf, err := buffer.NewGrid(ctx.Cols, 1)
	if err != nil {
		return measureOnlyBuf(ctx.Cols, 1)
	}
	writeRunes(buf, 0, 0, line, themeOrDefault(ctx.Theme).Style(StyleDimmed), ctx.Cols)
	return buf
}

// writeRunes writes text into buf at (x0,y) with style s, stopping before
// maxX (exclusive). Returns the end x. Wide-rune aware (render/width).
func writeRunes(buf *buffer.Grid, x0, y int, text string, s style.Style, maxX int) int {
	x := x0
	for _, r := range text {
		rw := width.RuneWidth(r)
		if rw <= 0 {
			continue
		}
		if x+rw > maxX {
			break
		}
		buf.SetCell(x, y, buffer.Cell{Ch: r, St: s})
		x += rw
	}
	return x
}
