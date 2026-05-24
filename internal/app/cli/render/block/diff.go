// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File diff.go — spec-1.13 D-1 + D-2 Diff block production (spec-0.13
// stub → 7th production block). Mirror CC FileEditToolDiff + StructuredPatchHunk
// shape per verified B-37; per-marker prefix style (NOT extraAttrs.FG
// row-overlay) per R2 CRIT-1 ★A; `-` line CC parity skip chroma highlight
// per R3 CRIT-2 ★A (B-38 color-diff/index.ts:915-918).

package block

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
	"github.com/sqlrush/opendbx/internal/app/cli/render/style"
)

// Diff is the spec-1.13 7th production block type. Renders unified-diff
// hunks with per-line marker classification ('+'/'-'/' ') + optional
// single-column gutter (CC computeGutterWidth B-39 parity per R2 HIGH-4
// ★A) + optional chroma syntax highlighting on body lines.
//
// Input modes (per Q5 ★A structured hunks primary):
//   - NewDiffFromHunks(hunks): primary; mirrors CC StructuredPatchHunk
//   - ParseUnified(text): D-6 parser; returns (Diff, error) for raw
//     `git diff -u` output (R2 MED-1 godoc fix)
//   - NewDiffFromBareLines(text): markdown ```diff fence path (no `@@` header)
//   - NewDiff(source, filePath): auto-detect; ParseUnified failure
//     fallback to bare-lines with slog.Warn (R2 NIT-3)
//
// Compile-time guarantee that Diff satisfies RenderNode (R2 LOW-2):
//
//	var _ RenderNode = Diff{}
type Diff struct {
	Hunks    []Hunk
	FilePath string // optional; extracted by ParseUnified or caller-supplied
	BodyLang string // chroma lang for body syntax highlight; empty = plain
}

// Hunk mirrors CC StructuredPatchHunk shape (from `diff` npm package
// per B-37): OldStart/OldLines/NewStart/NewLines are 1-based file line
// numbers with their respective change counts; Lines holds the marker-
// prefixed body entries.
//
// Bare-lines mode (markdown fence): OldStart == 0 signals "no gutter"
// and the hunk header `@@ -O,L +N,L @@` is NOT emitted in render.
type Hunk struct {
	OldStart, OldLines int
	NewStart, NewLines int
	Lines              []LineEntry
}

// LineEntry is one body line of a hunk, marker-classified.
// Marker ∈ {'+', '-', ' ', '?'}; '?' is the bare-lines mode sentinel
// (no gutter line numbers, no `@@` header).
type LineEntry struct {
	Marker rune
	Text   string
}

// Compile-time interface assertion (R2 LOW-2).
var _ RenderNode = Diff{}

// NewDiffFromHunks constructs a Diff from explicit structured hunks
// (primary API per Q5 ★A; spec-1.21 tool-call caller path).
func NewDiffFromHunks(hunks []Hunk) Diff {
	return Diff{Hunks: hunks}
}

// NewDiffFromBareLines synthesizes a single bare-lines hunk from a
// raw body of `+`/`-`/` `-prefixed lines (markdown ```diff fence path
// per D-5 + Q3 ★B). OldStart=0 → no gutter, no `@@` header.
func NewDiffFromBareLines(text string) Diff {
	var lines []LineEntry
	for _, line := range strings.Split(text, "\n") {
		if line == "" {
			continue
		}
		marker := classifyMarker(line)
		// Strip marker prefix from body (single byte for '+'/'-'/' ';
		// fallback '?' for malformed first-byte).
		body := line
		if marker != '?' && len(line) > 0 {
			body = line[1:]
		}
		lines = append(lines, LineEntry{Marker: marker, Text: body})
	}
	if len(lines) == 0 {
		return Diff{}
	}
	return Diff{
		Hunks: []Hunk{{OldStart: 0, Lines: lines}}, // OldStart=0 = no gutter
	}
}

// NewDiff is the auto-detect entry: tries ParseUnified first; falls back
// to NewDiffFromBareLines with slog.Warn on parse error (R2 NIT-3).
func NewDiff(source, filePath string) Diff {
	d, err := ParseUnified(source)
	if err == nil {
		if filePath != "" {
			d.FilePath = filePath
		}
		return d
	}
	slog.Warn("diff.NewDiff: ParseUnified failed; fallback bare-lines", "err", err, "filePath", filePath)
	d = NewDiffFromBareLines(source)
	d.FilePath = filePath
	return d
}

// classifyMarker returns the LineEntry marker from a raw body line's
// first byte. Returns '?' for lines that don't start with one of
// '+', '-', or ' ' (bare-lines mode tolerant fallback).
func classifyMarker(line string) rune {
	if line == "" {
		return '?'
	}
	switch line[0] {
	case '+', '-', ' ':
		return rune(line[0])
	default:
		return '?'
	}
}

// serializeDiff produces a deterministic byte representation of a Diff
// for cache-key sha256 (R2 MED-3 — empty Diff produces small string,
// large Diff naturally bypassed via blockCacheMaxSourceBytes guard).
// Format is stable across runs because Hunks is a slice (not a map).
func serializeDiff(d Diff) string {
	var b strings.Builder
	b.WriteString(d.FilePath)
	b.WriteByte('\n')
	b.WriteString(d.BodyLang)
	b.WriteByte('\n')
	for _, h := range d.Hunks {
		fmt.Fprintf(&b, "@@%d,%d+%d,%d@@\n", h.OldStart, h.OldLines, h.NewStart, h.NewLines)
		for _, ln := range h.Lines {
			b.WriteRune(ln.Marker)
			b.WriteString(ln.Text)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// Render produces the cell grid for the Diff. Pipeline:
//
//  1. ctx.Cols <= 0 → measureOnlyBuf(0, 0) fast path
//  2. cache lookup via 8-field blockCache key (source = serializeDiff)
//  3. for each hunk: emit `@@ -O,L +N,L @@` header (unless bare-lines
//     OldStart=0); for each LineEntry compute single-column gutter
//     (R2 HIGH-4 ★A CC parity) + prefix marker + body cells (highlight
//     when lang non-empty AND marker != '-' per R3 CRIT-2 ★A)
//  4. MeasureOnly returns row count without cell writes
//  5. blockCache.Put + return
func (d Diff) Render(ctx Context) (buffer.Buffer, error) {
	if ctx.Cols <= 0 {
		return measureOnlyBuf(0, 0), nil
	}
	theme := themeOrDefault(ctx.Theme)
	hl, _ := theme.(HighlighterTheme)
	codeStyleName := ""
	if hl != nil {
		codeStyleName = hl.CodeStyle()
	}

	source := serializeDiff(d)
	cacheable := !ctx.MeasureOnly && len(source) <= blockCacheMaxSourceBytes
	var cacheKey string
	if cacheable {
		// ctx.Verbose retained in cache key for shape parity with spec-1.11
		// Markdown / spec-1.12 Code (R2 LOW-1: Diff renders identically
		// regardless of Verbose today; keeping the key shape stable now
		// lets spec-1.21 verbose-header rows attach without invalidating
		// existing entries' key layout).
		cacheKey = makeBlockCacheKey(source, ctx.Cols, ctx.Verbose, themeCacheKey(theme), ctx.Wrap, d.BodyLang+":diff", codeStyleName, ctx.ColorDepth)
		if cached := blockCache.Get(cacheKey); cached != nil {
			return cached, nil
		}
	}

	rows := renderDiffRows(d, ctx, theme, hl)
	if len(rows) == 0 {
		return measureOnlyBuf(ctx.Cols, 0), nil
	}
	if ctx.MeasureOnly {
		return measureOnlyBuf(ctx.Cols, len(rows)), nil
	}
	grid, err := buffer.NewGrid(ctx.Cols, len(rows))
	if err != nil {
		// R2 MED-3: surface the alloc failure via slog so it's diagnosable
		// in production. Caller still sees a measure-only buffer (matches
		// spec-1.11 / spec-1.12 inherited behaviour); errcode escalation
		// would require a Render signature change across all 7 blocks.
		slog.Error("diff.Render: buffer.NewGrid failed", "cols", ctx.Cols, "rows", len(rows), "err", err)
		return measureOnlyBuf(ctx.Cols, len(rows)), nil
	}
	for y, row := range rows {
		writeDiffRow(grid, y, row, ctx.Cols, theme)
	}
	if cacheable {
		blockCache.Put(cacheKey, grid)
	}
	return grid, nil
}

// diffRow is render-time scratch for a single rendered line.
// prefix carries gutter + marker; cells is body content (chroma-styled
// or plain). prefixStyle is pre-resolved style.Style.
type diffRow struct {
	prefix      string
	prefixStyle style.Style
	cells       []buffer.Cell // body cells (post-marker); chroma when lang highlight
}

// renderDiffRows builds []diffRow for all hunks. Pure / no side effects.
// R2 HIGH-3: chroma highlight runs ONCE per hunk on the joined non-'-'
// body to preserve multi-line lexer context. R2 MED-4: rows capacity is
// pre-counted (1 header + len(Lines) per hunk that has gutter, len(Lines)
// otherwise).
func renderDiffRows(d Diff, ctx Context, theme StyleTheme, hl HighlighterTheme) []diffRow {
	total := 0
	for _, h := range d.Hunks {
		if h.OldStart > 0 {
			total++
		}
		total += len(h.Lines)
	}
	rows := make([]diffRow, 0, total)

	for _, h := range d.Hunks {
		// Emit hunk header (skip bare-lines mode OldStart=0).
		if h.OldStart > 0 {
			header := fmt.Sprintf("@@ -%d,%d +%d,%d @@", h.OldStart, h.OldLines, h.NewStart, h.NewLines)
			rows = append(rows, diffRow{
				prefix:      header,
				prefixStyle: theme.Style(StyleDiffHunkHeader),
			})
		}
		// R2 HIGH-3 batch highlight; nil result → per-line plain fallback.
		batch := highlightHunkBatch(h, d.BodyLang, ctx.ColorDepth, hl)
		oldNo, newNo := h.OldStart, h.NewStart
		gutterDigits := computeGutterDigits(h)
		for i, ln := range h.Lines {
			gutter := singleColumnGutter(ln.Marker, oldNo, newNo, gutterDigits, h.OldStart > 0)
			prefix := gutter + string(ln.Marker) + " "
			cells, ok := batch[i]
			if !ok || ln.Marker == '-' {
				cells = renderDiffHunkBody(ln.Marker, ln.Text, d.BodyLang, ctx, theme, hl)
			}
			rows = append(rows, diffRow{
				prefix:      prefix,
				prefixStyle: prefixStyleForMarker(ln.Marker, theme),
				cells:       cells,
			})
			// Advance line numbers per marker (R2 HIGH-4 ★A CC parity).
			switch ln.Marker {
			case '+':
				newNo++
			case '-':
				oldNo++
			case ' ':
				oldNo++
				newNo++
			}
		}
	}
	return rows
}

// computeGutterDigits returns the column width needed to hold the
// largest line number in this hunk (R2 HIGH-4 ★A; bare-lines = 0).
// R2 MED-5: digit count by base-10 loop, no fmt.Sprintf allocation.
func computeGutterDigits(h Hunk) int {
	if h.OldStart == 0 {
		return 0
	}
	maxNo := h.OldStart + h.OldLines - 1
	if n := h.NewStart + h.NewLines - 1; n > maxNo {
		maxNo = n
	}
	return digitCount(maxNo)
}

// digitCount returns the number of decimal digits needed to print n.
// Allocation-free (R2 MED-5 idiom).
func digitCount(n int) int {
	if n <= 0 {
		return 1
	}
	d := 0
	for n > 0 {
		d++
		n /= 10
	}
	return d
}

// singleColumnGutter formats the single-number gutter per marker
// (R2 HIGH-4 ★A + CC computeGutterWidth B-39 parity):
//
//	'+' → newNo (added line; newNo only relevant)
//	'-' → oldNo (removed line; oldNo only relevant)
//	' ' → newNo (context; both old+new same — display new)
//	'?' → empty padding (bare-lines mode)
//
// Returns "" when hasGutter is false (bare-lines mode signals digits=0).
func singleColumnGutter(marker rune, oldNo, newNo, digits int, hasGutter bool) string {
	if !hasGutter || digits == 0 {
		return ""
	}
	var n int
	switch marker {
	case '+':
		n = newNo
	case '-':
		n = oldNo
	case ' ':
		n = newNo
	default:
		return strings.Repeat(" ", digits) + " "
	}
	return fmt.Sprintf("%*d ", digits, n)
}

// prefixStyleForMarker returns the style.Style for the gutter+marker
// prefix column per R2 CRIT-1 ★A (marker color lives ONLY on prefix;
// body cells preserve chroma token colors).
func prefixStyleForMarker(marker rune, theme StyleTheme) style.Style {
	switch marker {
	case '+':
		return theme.Style(StyleDiffAdded)
	case '-':
		return theme.Style(StyleDiffRemoved)
	case ' ':
		return theme.Style(StyleDiffContext)
	default:
		return style.Style{}
	}
}

// writeDiffRow paints a single diffRow into the grid at row y.
// Prefix column is rendered with prefixStyle; body cells append after
// prefix. Body cells may be pre-styled (chroma) or plain (Style{}).
func writeDiffRow(grid *buffer.Grid, y int, row diffRow, cols int, theme StyleTheme) {
	x := 0
	for _, r := range row.prefix {
		if x >= cols {
			return
		}
		grid.SetCell(x, y, buffer.Cell{Ch: r, St: row.prefixStyle})
		x++
	}
	for _, c := range row.cells {
		if x >= cols {
			break
		}
		if c.Ch == 0 {
			x++
			continue
		}
		grid.SetCell(x, y, c)
		x++
	}
}
