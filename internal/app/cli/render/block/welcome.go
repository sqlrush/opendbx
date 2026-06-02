// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File welcome.go — production Welcome panel block (spec-1.25 D-1 + errata R4).
//
// !!! TEST-PHASE ONLY — MUST BE REPLACED BEFORE 1.0 COMMERCIALIZATION !!!
// errata R4 (用户 path 3/3 2026-06-02): replicates Claude Code v2.1.x's
// two-panel rounded-box welcome (left: greeting + sparkle mascot + model +
// cwd; right: tips / what's new) for the TEST PHASE, to validate render-engine
// fidelity + CC UX parity. The sparkle mascot + layout mirror Anthropic's CC;
// per the user directive they MUST be swapped for an opendbx-original welcome
// before commercial 1.0 (tracked: spec-1.25 §10 + roadmap pre-1.0 gate;
// CLAUDE.md § 3.2). Do NOT ship this welcome in a commercial build.
//
// render-only: never enters the llm wire.

package block

import (
	"strings"

	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
	"github.com/sqlrush/opendbx/internal/app/cli/render/style"
	"github.com/sqlrush/opendbx/internal/app/cli/render/width"
)

const (
	// welcomeLeftW is the left-panel inner width (CC v2.1.x uses 52).
	welcomeLeftW = 52
	// welcomeMinCols is the minimum terminal width for the two-panel box;
	// below it the welcome degrades to a single text line.
	welcomeMinCols = 90
)

// welcomeMascot is CC's minimal sparkle mark (quadrant block runes), centered
// in the left panel. TEST-PHASE clone — see file header.
var welcomeMascot = []string{
	"▗ ▗   ▖ ▖",
	"▘▘ ▝▝",
}

// welcomeAccent is the Claude-brand terracotta orange (~#D97757), used for the
// title + sparkle mascot (TEST-PHASE clone fidelity).
var welcomeAccent = style.Style{FG: style.RGB(0xD9, 0x77, 0x57)}

// Welcome is the spec-1.25 D-1 production welcome panel (replaces the
// spec-0.13 Banner stub). Zero-value renders with empty fields.
type Welcome struct {
	Version string // build version (internal/platform/version.String())
	Cwd     string // working dir, already ~-abbreviated by the caller
	Tip     string // single static onboarding tip (non-random; replayable)
	Model   string // active model name (left-panel line)
}

// NewWelcome constructs a Welcome panel.
func NewWelcome(version, cwd, tip string) Welcome {
	return Welcome{Version: version, Cwd: cwd, Tip: tip}
}

// Render produces the CC v2.1.x two-panel rounded-box welcome. Graceful:
// Cols < welcomeMinCols → single-line `✻ Welcome to opendbx <version>`.
// MeasureOnly returns the row count with no cell writes.
func (w Welcome) Render(ctx Context) (buffer.Buffer, error) {
	if ctx.Cols <= 0 {
		return measureOnlyBuf(0, 0), nil
	}
	if ctx.Cols < welcomeMinCols {
		return w.renderFallback(ctx), nil
	}
	left := w.leftPanel()
	right := w.rightPanel()
	n := max(len(left), len(right))
	rows := n + 2 // top + content + bottom
	if ctx.MeasureOnly {
		return measureOnlyBuf(ctx.Cols, rows), nil
	}
	buf, err := buffer.NewGrid(ctx.Cols, rows)
	if err != nil {
		return measureOnlyBuf(ctx.Cols, rows), nil
	}
	theme := themeOrDefault(ctx.Theme)
	dim := theme.Style(StyleDimmed)
	cols := ctx.Cols

	w.paintTop(buf, cols, dim)
	for i := 0; i < n; i++ {
		y := i + 1
		buf.SetCell(0, y, buffer.Cell{Ch: '│', St: dim})
		buf.SetCell(welcomeLeftW+1, y, buffer.Cell{Ch: '│', St: dim})
		buf.SetCell(cols-1, y, buffer.Cell{Ch: '│', St: dim})
		if i < len(left) {
			lx := 1 + center(left[i].text, welcomeLeftW)
			writeRunes(buf, lx, y, left[i].text, left[i].st, welcomeLeftW+1)
		}
		if i < len(right) {
			writeRunes(buf, welcomeLeftW+3, y, right[i].text, right[i].st, cols-1)
		}
	}
	w.paintBottom(buf, rows-1, cols, dim)
	return buf, nil
}

// styledLine is a panel line with its style.
type styledLine struct {
	text string
	st   style.Style
}

// leftPanel builds the centered left-panel lines (greeting + mascot + model
// + cwd), styled.
func (w Welcome) leftPanel() []styledLine {
	dim := DefaultTheme{}.Style(StyleDimmed)
	normal := DefaultTheme{}.Style(StyleNormal)
	out := []styledLine{
		{"", normal},
		{"Welcome to opendbx!", normal},
		{"", normal},
		{welcomeMascot[0], welcomeAccent},
		{"", normal},
		{welcomeMascot[1], welcomeAccent},
	}
	if w.Model != "" {
		out = append(out, styledLine{w.Model, dim})
	} else {
		out = append(out, styledLine{"", normal})
	}
	out = append(out, styledLine{"", normal})
	if w.Cwd != "" {
		out = append(out, styledLine{w.Cwd, dim})
	}
	return out
}

// rightPanel builds the left-aligned right-panel lines (tips / what's new),
// styled.
func (w Welcome) rightPanel() []styledLine {
	dim := DefaultTheme{}.Style(StyleDimmed)
	normal := DefaultTheme{}.Style(StyleNormal)
	tip := w.Tip
	if tip == "" {
		tip = "直接用自然语言描述你的数据库问题即可开始诊断"
	}
	return []styledLine{
		{"", normal},
		{"Tips for getting started", normal},
		{tip, dim},
		{strings.Repeat("─", 40), dim},
		{"What's new", normal},
		{"interact TUI 对齐 Claude Code(spec-1.25)", dim},
		{"welcome / 输入框 / 状态行 / ⏺ 消息 / ⎿ 工具树", dim},
		{"", normal},
		{"自然语言诊断 · \\ 执行 SQL · / 命令(Stage 2)", dim},
	}
}

// paintTop writes ╭─── opendbx <version> ───…───╮ across cols.
func (w Welcome) paintTop(buf *buffer.Grid, cols int, dim style.Style) {
	title := "opendbx"
	if w.Version != "" {
		title += " " + w.Version
	}
	buf.SetCell(0, 0, buffer.Cell{Ch: '╭', St: dim})
	x := 1
	for ; x < 4 && x < cols-1; x++ {
		buf.SetCell(x, 0, buffer.Cell{Ch: '─', St: dim})
	}
	if x < cols-1 {
		buf.SetCell(x, 0, buffer.Cell{Ch: ' ', St: dim})
		x++
	}
	x = writeRunes(buf, x, 0, title, welcomeAccent, cols-1)
	if x < cols-1 {
		buf.SetCell(x, 0, buffer.Cell{Ch: ' ', St: dim})
		x++
	}
	for ; x < cols-1; x++ {
		buf.SetCell(x, 0, buffer.Cell{Ch: '─', St: dim})
	}
	buf.SetCell(cols-1, 0, buffer.Cell{Ch: '╮', St: dim})
}

// paintBottom writes ╰───…───╯ at row y.
func (w Welcome) paintBottom(buf *buffer.Grid, y, cols int, dim style.Style) {
	buf.SetCell(0, y, buffer.Cell{Ch: '╰', St: dim})
	for x := 1; x < cols-1; x++ {
		buf.SetCell(x, y, buffer.Cell{Ch: '─', St: dim})
	}
	buf.SetCell(cols-1, y, buffer.Cell{Ch: '╯', St: dim})
}

// center returns the left-pad (in cells) to center text of display width
// within w columns.
func center(text string, w int) int {
	tw := width.Width(text)
	if tw >= w {
		return 0
	}
	return (w - tw) / 2
}

// renderFallback returns a single dim line when the terminal is too narrow
// for the two-panel box.
func (w Welcome) renderFallback(ctx Context) buffer.Buffer {
	if ctx.MeasureOnly {
		return measureOnlyBuf(ctx.Cols, 1)
	}
	line := "✻ Welcome to opendbx"
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
