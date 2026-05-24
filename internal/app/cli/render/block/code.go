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

// Code is the spec-1.12 D-4 production code block. Caller-invoked top-
// level code snippet renderer (when content type is "this is a code
// snippet" not markdown-embedded fence). Internal: delegates to the
// shared renderCodeBlock helper (spec-1.7 D-5 + spec-1.12 D-3 in-place
// upgrade) so all three callers (Message fence / Markdown fence /
// Code block) share highlighter + cache + downgrade paths.
//
// **spec-1.7 R2 D6 HIGH-C double duty**: code.go hosts (a) the Code
// production block; (b) private helper `renderCodeBlock` used by
// block.Message.Render fence path (spec-1.7 D-4) and block.walker
// markdown fence (spec-1.11 D-6). spec-1.12 promoted Code from stub
// to production.
//
// Lang empty → plain path (no chroma overhead). Lang non-empty + theme
// implements HighlighterTheme → chroma syntax highlight via D-2/D-3.
// No state field — highlighter LRU cache is package-level singleton
// (blockCache shared with spec-1.11 markdown per Q6 ★B).
type Code struct {
	Source string // full code body (caller-supplied snapshot)
	Lang   string // chroma lang name or alias (e.g., "go", "python", "bash"); empty → plain
}

// NewCode returns a Code block with the given source and lang.
func NewCode(source, lang string) Code { return Code{Source: source, Lang: lang} }

// Render delegates to renderCodeBlock (spec-1.7 D-5 / spec-1.12 D-3).
// spec-1.12 D-4: thin wrapper preserves the spec-1.7 helper signature
// + cache + highlight + fallback chain.
func (c Code) Render(ctx Context) (buffer.Buffer, error) {
	if ctx.Cols <= 0 {
		return measureOnlyBuf(0, 0), nil
	}
	buf, _ := renderCodeBlock(ctx, c.Lang, c.Source)
	return buf, nil
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
//
// **spec-1.12 D-3 in-place upgrade (rule 21)**: when lang != "" AND
// ctx.Theme implements HighlighterTheme, attempts chroma syntax
// highlight via highlight(); failure / chroma panic / unknown lang
// silently falls back to the plain monospace path below (I-2 preserves
// spec-1.7 contract: Buffer always non-nil). Cache 8-field key
// (source/cols/verbose/themeKey/wrap/lang/codeStyleName/colorDepth)
// shared with spec-1.11 markdown via blockCache singleton (Q6 ★B).
func renderCodeBlock(ctx Context, lang, body string) (buffer.Buffer, int) {
	theme := themeOrDefault(ctx.Theme)

	// spec-1.12 D-7 cache lookup: skip when source too big or MeasureOnly
	// (MeasureOnly fast path below uses computed row count directly).
	cacheable := !ctx.MeasureOnly && len(body) <= blockCacheMaxSourceBytes
	var cacheKey string
	if cacheable {
		styleName := ""
		if hl, ok := ctx.Theme.(HighlighterTheme); ok {
			styleName = hl.CodeStyle()
		}
		cacheKey = makeBlockCacheKey(body, ctx.Cols, ctx.Verbose, themeCacheKey(theme), ctx.Wrap, lang, styleName, ctx.ColorDepth)
		if cached := blockCache.Get(cacheKey); cached != nil {
			_, r := cached.Size()
			return cached, r
		}
	}

	// spec-1.12 D-3 highlight dispatch: when lang non-empty AND theme
	// implements HighlighterTheme, try chroma. Panic / unknown lang
	// falls through to plain path (I-2).
	if hl, ok := ctx.Theme.(HighlighterTheme); ok && strings.TrimSpace(lang) != "" {
		if buf, rows := renderHighlightedCodeBlock(ctx, lang, body, hl); buf != nil {
			if cacheable {
				blockCache.Put(cacheKey, buf)
			}
			return buf, rows
		}
	}

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

// renderHighlightedCodeBlock attempts chroma syntax highlight (spec-1.12
// D-3). Returns (nil, 0) on highlight failure so renderCodeBlock falls
// back to plain path (I-2 contract). Layout matches plain path: row 0
// is the lang label, body rows start at y=1 with 1-cell left/right
// padding + per-token styled cells. Bold/Italic/Underline preserved
// from chroma StyleEntry; FG/BG from D-6 mapChromaColor downgrade.
func renderHighlightedCodeBlock(ctx Context, lang, body string, hl HighlighterTheme) (buffer.Buffer, int) {
	rows, err := highlight(lang, body, hl, ctx.ColorDepth)
	if err != nil || rows == nil {
		return nil, 0
	}
	totalRows := 1 + len(rows) // 1 lang label + body rows
	if ctx.MeasureOnly {
		return measureOnlyBuf(ctx.Cols, totalRows), totalRows
	}
	buf, gerr := buffer.NewGrid(ctx.Cols, totalRows)
	if gerr != nil {
		return nil, 0
	}
	theme := themeOrDefault(ctx.Theme)
	// Row 0: lang label (same visual as plain path).
	labelStyle := theme.Style(StyleLangLabel)
	label := buildLangLabel(lang, ctx.Cols)
	for x, r := range []rune(label) {
		if x >= ctx.Cols {
			break
		}
		buf.SetCell(x, 0, buffer.Cell{Ch: r, St: labelStyle})
	}
	// Body rows: write per-token styled cells starting at x=1 (1-cell
	// left padding). 1-cell right padding preserved by overflow break.
	for i, hrow := range rows {
		y := 1 + i
		x := 1
		for _, c := range hrow.cells {
			rw := width.RuneWidth(c.Ch)
			if rw <= 0 {
				continue
			}
			if x+rw > ctx.Cols-1 {
				break
			}
			buf.SetCell(x, y, c)
			x += rw
		}
	}
	return buf, totalRows
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
