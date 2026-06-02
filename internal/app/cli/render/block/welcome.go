// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File welcome.go — production Welcome panel block (spec-1.25 D-1 + errata R4).
//
// !!! TEST-PHASE ONLY — MUST BE REPLACED BEFORE 1.0 COMMERCIALIZATION !!!
// errata R4 (用户 path 3/3 2026-06-02): the user authorized a 1:1 clone of CC's
// WelcomeV2 dark-variant welcome (including the Claude/clawd mascot ASCII-art)
// FOR THE TEST PHASE ONLY, to validate render-engine fidelity + CC UX parity.
// The mascot art below is Anthropic's brand asset; per the user directive it
// MUST be swapped for an opendbx-original logo before commercial 1.0 (tracked:
// spec-1.25 §10 forward-link + roadmap pre-1.0 gate; CLAUDE.md § 3.2). Do NOT
// ship this welcome in a commercial build.
//
// render-only: never enters the llm wire.

package block

import (
	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
	"github.com/sqlrush/opendbx/internal/app/cli/render/style"
	"github.com/sqlrush/opendbx/internal/app/cli/render/width"
)

// welcomeWidth is CC's WELCOME_V2_WIDTH (WelcomeV2.tsx:5). Below it the
// welcome degrades to a single text line.
const welcomeWidth = 58

// welcomeTitle is the headline (CC dark-variant format: title + dim version).
const welcomeTitle = "Welcome to opendbx"

// welcomeArt is CC's WelcomeV2 dark-variant mascot (sunset rays `░` + clawd
// body `█▓▒` + sparkles `*`, 58-wide), extracted verbatim from
// claude-code-source-code WelcomeV2.tsx t1..t15. TEST-PHASE clone — see file
// header (MUST replace before 1.0). Per-glyph coloring approximates CC's
// claude/clawd spans (body=accent orange, rays/horizon=dim).
var welcomeArt = []string{
	"…………………………………………………………………………………………………………………………………………………………",
	"                                                          ",
	"     *                                       █████▓▓░     ",
	"                                 *         ███▓░     ░░   ",
	"            ░░░░░░                        ███▓░           ",
	"    ░░░   ░░░░░░░░░░                      ███▓░           ",
	"   ░░░░░░░░░░░░░░░░░░░                    ██▓░░      ▓   ",
	"                                             ░▓▓███▓▓░    ",
	" *                                 ░░░░                   ",
	"                                 ░░░░░░░░                 ",
	"                               ░░░░░░░░░░░░░░░░           ",
	"",
	"                                             ",
	"                                              ",
	"           *                                   ",
}

// welcomeAccent is the Claude-brand terracotta orange (~#D97757) used for the
// title + mascot body glyphs (TEST-PHASE clone fidelity).
var welcomeAccent = style.Style{FG: style.RGB(0xD9, 0x77, 0x57)}

// Welcome is the spec-1.25 D-1 production welcome panel (replaces the
// spec-0.13 Banner stub). Zero-value Welcome{} renders with no version/cwd.
type Welcome struct {
	Version string // build version (internal/platform/version.String())
	Cwd     string // working dir, already ~-abbreviated by the caller
	Tip     string // single static onboarding tip (non-random; replayable)
}

// NewWelcome constructs a Welcome panel.
func NewWelcome(version, cwd, tip string) Welcome {
	return Welcome{Version: version, Cwd: cwd, Tip: tip}
}

// Render produces the CC dark-variant welcome: title line, then the clawd
// mascot art, then a dim cwd/tip footer. Graceful: Cols < welcomeWidth →
// single-line `✻ Welcome to opendbx <version>` fallback. MeasureOnly returns
// the row count with no cell writes.
func (w Welcome) Render(ctx Context) (buffer.Buffer, error) {
	if ctx.Cols <= 0 {
		return measureOnlyBuf(0, 0), nil
	}
	if ctx.Cols < welcomeWidth {
		return w.renderFallback(ctx), nil
	}
	footer := w.footerLines()
	rows := 1 + len(welcomeArt) // title + mascot
	if len(footer) > 0 {
		rows += 1 + len(footer) // blank + footer
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

	// Title: "Welcome to opendbx" (accent) + " <version>" (dim).
	x := writeRunes(buf, 0, 0, welcomeTitle, welcomeAccent, ctx.Cols)
	if w.Version != "" {
		writeRunes(buf, x, 0, " "+w.Version, dim, ctx.Cols)
	}
	// Mascot art, per-glyph colored.
	for i, line := range welcomeArt {
		writeArtRow(buf, i+1, line, dim, ctx.Cols)
	}
	// Footer (cwd / tip), dim, after a blank row.
	for i, line := range footer {
		writeRunes(buf, 0, 1+len(welcomeArt)+1+i, line, dim, ctx.Cols)
	}
	return buf, nil
}

// footerLines builds the optional dim cwd/tip lines shown under the mascot.
func (w Welcome) footerLines() []string {
	var out []string
	if w.Cwd != "" {
		out = append(out, "cwd: "+w.Cwd)
	}
	if w.Tip != "" {
		out = append(out, w.Tip)
	}
	return out
}

// writeArtRow paints one mascot row with per-glyph coloring: body glyphs
// (█▓▒) + sparkle (*) use the accent; rays/horizon (░ …) use dim; spaces are
// blank. Stops at Cols.
func writeArtRow(buf *buffer.Grid, y int, line string, dim style.Style, cols int) {
	x := 0
	for _, r := range line {
		rw := width.RuneWidth(r)
		if rw <= 0 {
			continue
		}
		if x+rw > cols {
			break
		}
		switch r {
		case ' ':
			// leave blank
		case '█', '▓', '▒', '*':
			buf.SetCell(x, y, buffer.Cell{Ch: r, St: welcomeAccent})
		default: // '░', '…' and any other → dim
			buf.SetCell(x, y, buffer.Cell{Ch: r, St: dim})
		}
		x += rw
	}
}

// renderFallback returns a single dim line "✻ Welcome to opendbx <version>"
// when the terminal is too narrow for the 58-wide mascot.
func (w Welcome) renderFallback(ctx Context) buffer.Buffer {
	if ctx.MeasureOnly {
		return measureOnlyBuf(ctx.Cols, 1)
	}
	line := "✻ " + welcomeTitle
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
