// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File welcome.go — production Welcome panel block (spec-1.25 D-1).
// Renames + replaces the spec-0.13 Banner stub (which returned
// ErrUnsupportedNode; grep-confirmed zero production callers). Renders the
// classic CC-style boxed welcome `╭─ ✻ Welcome to opendbx ─╮` — a
// deliberate opendbx approximation of the pre-LogoV2 CC welcome (CC's
// current WelcomeV2.tsx is an ASCII-art logo; spec-1.25 §1.1 #5 keeps the
// boxed form). `✻` = CC figures.ts:6 TEARDROP_ASTERISK. The body carries
// the version, cwd, a single static tip, and the "? for shortcuts" hint
// (CC PromptInputFooterLeftSide.tsx:411). render-only: never enters the
// llm wire.

package block

import (
	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
	"github.com/sqlrush/opendbx/internal/app/cli/render/style"
	"github.com/sqlrush/opendbx/internal/app/cli/render/width"
)

// welcomeTitle is the boxed-welcome title text (embedded in the top rule).
const welcomeTitle = "✻ Welcome to opendbx"

// welcomeShortcuts mirrors CC's "? for shortcuts" footer hint.
const welcomeShortcuts = "? for shortcuts"

// Box-drawing runes for the rounded welcome frame.
const (
	bdTopLeft     = '╭'
	bdTopRight    = '╮'
	bdBottomLeft  = '╰'
	bdBottomRight = '╯'
	bdHorizontal  = '─'
	bdVertical    = '│'
)

// Welcome is the spec-1.25 D-1 production welcome panel (replaces the
// spec-0.13 Banner stub). Zero-value Welcome{} renders a minimal frame
// (it does NOT return ErrUnsupportedNode — that was the stub contract).
type Welcome struct {
	Version string // build version (internal/platform/version.String())
	Cwd     string // working dir, already ~-abbreviated by the caller
	Tip     string // single static onboarding tip (non-random; replayable)
}

// NewWelcome constructs a Welcome panel. Empty fields are simply omitted
// from the body (e.g. version="" drops the version line).
func NewWelcome(version, cwd, tip string) Welcome {
	return Welcome{Version: version, Cwd: cwd, Tip: tip}
}

// Render produces the boxed welcome Buffer. Graceful degradation: when
// Cols is too small for the frame it falls back to a single truncated
// line `✻ Welcome to opendbx <version>` (spec-1.25 D-1 / R-2). MeasureOnly
// returns the correct row count with no cell writes (spec-1.7 contract).
func (w Welcome) Render(ctx Context) (buffer.Buffer, error) {
	if ctx.Cols <= 0 {
		return measureOnlyBuf(0, 0), nil
	}
	body := w.bodyLines()
	boxW, ok := boxWidth(body, ctx.Cols)
	if !ok {
		return w.renderFallback(ctx), nil
	}
	rows := len(body) + 2 // top rule + body + bottom rule
	if ctx.MeasureOnly {
		return measureOnlyBuf(ctx.Cols, rows), nil
	}
	theme := themeOrDefault(ctx.Theme)
	buf, err := buffer.NewGrid(ctx.Cols, rows)
	if err != nil {
		return measureOnlyBuf(ctx.Cols, rows), nil
	}
	dim := theme.Style(StyleDimmed)
	normal := theme.Style(StyleNormal)

	paintWelcomeTop(buf, boxW, dim, normal)
	inner := boxW - 4 // content width between "│ " and " │"
	for i, line := range body {
		paintWelcomeBody(buf, i+1, boxW, inner, line, dim)
	}
	paintWelcomeBottom(buf, rows-1, boxW, dim)
	return buf, nil
}

// bodyLines builds the ordered, non-empty body content lines.
func (w Welcome) bodyLines() []string {
	var lines []string
	if w.Version != "" {
		lines = append(lines, w.Version)
	}
	if w.Cwd != "" {
		lines = append(lines, w.Cwd)
	}
	if w.Tip != "" {
		lines = append(lines, w.Tip)
	}
	lines = append(lines, "") // blank spacer before the shortcuts hint
	lines = append(lines, welcomeShortcuts)
	return lines
}

// renderFallback returns a single dim line "✻ Welcome to opendbx <version>"
// truncated to Cols, used when the terminal is too narrow for the frame.
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

// boxWidth computes the frame width capped at cols. Returns ok=false when
// cols cannot fit even the minimal title frame (caller falls back to a
// single line). The frame must satisfy both the title rule (>= title+5:
// ╭─ title ─╮) and the widest body row (>= body+4: │ body │).
func boxWidth(body []string, cols int) (int, bool) {
	titleMin := width.Width(welcomeTitle) + 5
	bodyMin := 0
	for _, l := range body {
		if bw := width.Width(l) + 4; bw > bodyMin {
			bodyMin = bw
		}
	}
	want := titleMin
	if bodyMin > want {
		want = bodyMin
	}
	if cols < titleMin {
		return 0, false // too narrow even for the title frame
	}
	if want > cols {
		want = cols // cap; body content truncates via writeRunes
	}
	return want, true
}

// paintWelcomeTop writes ╭─ <title> ─...─╮ across boxW cells. The title is
// StyleNormal; the frame runes are dim.
func paintWelcomeTop(buf *buffer.Grid, boxW int, dim, normal style.Style) {
	buf.SetCell(0, 0, buffer.Cell{Ch: bdTopLeft, St: dim})
	buf.SetCell(1, 0, buffer.Cell{Ch: bdHorizontal, St: dim})
	buf.SetCell(2, 0, buffer.Cell{Ch: ' ', St: dim})
	x := writeRunes(buf, 3, 0, welcomeTitle, normal, boxW-1)
	if x < boxW-1 {
		buf.SetCell(x, 0, buffer.Cell{Ch: ' ', St: dim})
		x++
	}
	for ; x < boxW-1; x++ {
		buf.SetCell(x, 0, buffer.Cell{Ch: bdHorizontal, St: dim})
	}
	buf.SetCell(boxW-1, 0, buffer.Cell{Ch: bdTopRight, St: dim})
}

// paintWelcomeBody writes │ <content padded to inner> │ at row y.
func paintWelcomeBody(buf *buffer.Grid, y, boxW, inner int, content string, dim style.Style) {
	buf.SetCell(0, y, buffer.Cell{Ch: bdVertical, St: dim})
	buf.SetCell(1, y, buffer.Cell{Ch: ' ', St: dim})
	end := writeRunes(buf, 2, y, content, dim, 2+inner)
	for x := end; x < boxW-1; x++ {
		buf.SetCell(x, y, buffer.Cell{Ch: ' ', St: dim})
	}
	buf.SetCell(boxW-1, y, buffer.Cell{Ch: bdVertical, St: dim})
}

// paintWelcomeBottom writes ╰─...─╯ across boxW cells at row y.
func paintWelcomeBottom(buf *buffer.Grid, y, boxW int, dim style.Style) {
	buf.SetCell(0, y, buffer.Cell{Ch: bdBottomLeft, St: dim})
	for x := 1; x < boxW-1; x++ {
		buf.SetCell(x, y, buffer.Cell{Ch: bdHorizontal, St: dim})
	}
	buf.SetCell(boxW-1, y, buffer.Cell{Ch: bdBottomRight, St: dim})
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
