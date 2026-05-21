// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package block

import (
	"strings"
	"unicode/utf8"

	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
	"github.com/sqlrush/opendbx/internal/app/cli/render/style"
	"github.com/sqlrush/opendbx/internal/app/cli/render/width"
)

// Message is the spec-0.13 D-3 message block type. spec-1.7 D-2 ships
// production Render per spec-1.6 forward (R-9 4 fields + R-12 robust
// fence + R2 D6 raw markers + R2 D11 Continued + 痛点 1.5 Empty).
//
// 4 fields (spec-1.6 R2 D2 inline errata to spec-0.13 D-3):
//   - Text:      raw token content from spec-1.6 TokenStream.
//     Contains literal ``` fence markers per R2 D6;
//     spec-1.7 Render identifies + strips fences + applies code-style.
//   - Truncated: FinishLength / FinishError / FinishCancelled paths;
//     renderer shows "…" + StyleWarning at last row tail.
//   - Continued: lineBuf cap overflow forced emit; R2 D11 semantics:
//     no trailing newline + "…" + StyleDimmed at last row tail;
//     next block in scrollback is logically continuous.
//   - Empty:     thinking-only placeholder (痛点 1.5); renderer shows
//     "(no output)" + StyleDimmed.
//
// **R2 D1 CRIT-1 marker rune**: Truncated and Continued share rune "…";
// Style distinguishes (StyleWarning for failure vs StyleDimmed for
// continuation), matching spec-1.6 FROZEN godoc.
type Message struct {
	Text      string
	Truncated bool
	Continued bool
	Empty     bool
}

// Render produces a Buffer per spec-1.7 D-2 (R2 forward batch).
//
// 4-branch dispatch:
//   - Empty=true              → "(no output)" placeholder (痛点 1.5)
//   - 0 fences detected       → renderPlainText (wrap + style)
//   - ≥1 fences detected      → renderMixed segment stitching
//     (markers apply to STITCHED buffer after concat per R2 D6)
//
// MeasureOnly fast path: returns Buffer with correct Size().rows but
// cells are no-op (spec-1.5 R-2 forward + spec-1.6 R-9).
func (m Message) Render(ctx Context) (buffer.Buffer, error) {
	if ctx.Cols <= 0 {
		return measureOnlyBuf(0, 0), nil
	}
	if m.Empty {
		return emptyPlaceholder(ctx), nil
	}
	if m.Text == "" {
		// Empty Text but Empty=false: 0-row Buffer per spec-1.7 R2 D6
		// HIGH-E (buffer.NewGrid rejects rows≤0; use measureOnlyBuf).
		return measureOnlyBuf(ctx.Cols, 0), nil
	}
	fences := scanFenceRanges(m.Text)
	if len(fences) == 0 {
		return renderPlainText(ctx, m), nil
	}
	return renderMixed(ctx, m, fences), nil
}

// renderPlainText renders Text with no fence detection. Applies
// WrapPolicy then optionally appends Truncated/Continued marker.
func renderPlainText(ctx Context, m Message) buffer.Buffer {
	theme := themeOrDefault(ctx.Theme)
	lines := wrap(m.Text, ctx.Cols, ctx.Wrap)
	rows := rowsWithMarker(m.Text, ctx.Cols, ctx.Wrap, m)
	if rows == 0 {
		return measureOnlyBuf(ctx.Cols, 0)
	}
	if ctx.MeasureOnly {
		return measureOnlyBuf(ctx.Cols, rows)
	}
	buf, err := buffer.NewGrid(ctx.Cols, rows)
	if err != nil {
		return measureOnlyBuf(ctx.Cols, rows)
	}
	normalStyle := theme.Style(StyleNormal)
	for i, line := range lines {
		writeTextRow(buf, 0, i, line, normalStyle, ctx.Cols)
	}
	// Truncated 优先 over Continued (R2 D6 Q5 子项 ★A).
	// Wrap=None already appends "…" internally — skip Truncated marker
	// double-add per R2 D6.
	if m.Truncated && ctx.Wrap != WrapNone {
		applyTruncated(buf, theme)
	} else if m.Continued && !m.Truncated {
		applyContinued(buf, theme)
	}
	return buf
}

// renderMixed handles Text containing one or more fence ranges.
// Algorithm (spec-1.7 R2 D6 HIGH-D explicit):
//  1. splitSegments — partition Text lines into ordered segProse |
//     segFence segments.
//  2. For each segment: renderSegment (no marker) → segment Buffer + rows.
//  3. stitchSegments — vertically concatenate segment Buffers into one
//     final Buffer with totalRows.
//  4. Apply Truncated/Continued marker to the STITCHED buffer last row
//     (not per-segment; per R2 D6 / claude HIGH-3).
//
// MeasureOnly fast path: skip per-segment cell rendering; sum rows only.
func renderMixed(ctx Context, m Message, fences []fenceRange) buffer.Buffer {
	segments := splitSegments(m.Text, fences)
	theme := themeOrDefault(ctx.Theme)

	// Collect each segment's Buffer + rows.
	type rendered struct {
		buf  buffer.Buffer
		rows int
	}
	rs := make([]rendered, 0, len(segments))
	totalRows := 0
	for _, seg := range segments {
		switch seg.kind {
		case segProse:
			proseText := seg.text
			proseMsg := Message{Text: proseText} // no marker; applied to stitched
			proseRows := rowsWithMarker(proseText, ctx.Cols, ctx.Wrap, proseMsg)
			if proseRows == 0 {
				continue
			}
			if ctx.MeasureOnly {
				rs = append(rs, rendered{measureOnlyBuf(ctx.Cols, proseRows), proseRows})
				totalRows += proseRows
				continue
			}
			rs = append(rs, rendered{renderPlainText(ctx, proseMsg), proseRows})
			totalRows += proseRows
		case segFence:
			fbuf, frows := renderCodeBlock(ctx, seg.lang, seg.text)
			rs = append(rs, rendered{fbuf, frows})
			totalRows += frows
		}
	}
	// Account for marker tail overflow on stitched buffer.
	markerExtra := 0
	if (m.Truncated || m.Continued) && totalRows > 0 {
		// Conservative: if last segment may overflow, +1 row.
		// We can refine via last-row width check after stitch.
		markerExtra = 0 // applyTailMarker handles overflow inline if room exists
	}
	totalRows += markerExtra
	if ctx.MeasureOnly {
		return measureOnlyBuf(ctx.Cols, totalRows)
	}
	if totalRows == 0 {
		return measureOnlyBuf(ctx.Cols, 0)
	}
	stitched, err := buffer.NewGrid(ctx.Cols, totalRows)
	if err != nil {
		return measureOnlyBuf(ctx.Cols, totalRows)
	}
	y := 0
	for _, r := range rs {
		// Copy r.buf into stitched starting at row y.
		grid, ok := r.buf.(*buffer.Grid)
		if !ok {
			y += r.rows // measure-only segment in real path shouldn't happen, but tolerate
			continue
		}
		_, srcRows := grid.Size()
		for sy := 0; sy < srcRows && y+sy < totalRows; sy++ {
			for sx := 0; sx < ctx.Cols; sx++ {
				stitched.SetCell(sx, y+sy, grid.Cell(sx, sy))
			}
		}
		y += r.rows
	}
	if m.Truncated {
		applyTruncated(stitched, theme)
	} else if m.Continued {
		applyContinued(stitched, theme)
	}
	return stitched
}

// segKind enumerates the segment types in renderMixed.
type segKind int

const (
	segProse segKind = iota
	segFence
)

// segment is a portion of Message.Text classified by kind.
type segment struct {
	kind segKind
	text string // raw text without leading/trailing newlines (prose);
	// for fence: body between opening + closing markers
	lang string // only meaningful for segFence
}

// splitSegments partitions text into ordered segments interleaving
// prose and fence per the fenceRange slice. Empty prose between
// consecutive fences is dropped.
func splitSegments(text string, fences []fenceRange) []segment {
	lines := strings.Split(text, "\n")
	var segs []segment
	cur := 0
	for _, f := range fences {
		if f.Start > cur {
			// Prose segment from line cur..f.Start-1.
			prose := strings.Join(lines[cur:f.Start], "\n")
			if strings.TrimSpace(prose) != "" {
				segs = append(segs, segment{kind: segProse, text: prose})
			}
		}
		// Fence body: lines (f.Start+1)..(f.End-1) if Closed; or
		// (f.Start+1)..(end-of-text) if unclosed.
		bodyStart := f.Start + 1
		bodyEnd := f.End
		if f.Closed {
			// End is the closing fence line itself; body excludes it.
		} else {
			// Unclosed: body runs to len(lines)-1 inclusive.
			bodyEnd = len(lines)
		}
		var bodyLines []string
		if bodyStart <= bodyEnd && bodyStart < len(lines) {
			if f.Closed {
				bodyLines = lines[bodyStart:f.End]
			} else {
				bodyLines = lines[bodyStart:]
			}
		}
		body := strings.Join(bodyLines, "\n")
		segs = append(segs, segment{kind: segFence, text: body, lang: f.Lang})
		if f.Closed {
			cur = f.End + 1
		} else {
			cur = len(lines)
		}
	}
	// Trailing prose after last fence.
	if cur < len(lines) {
		prose := strings.Join(lines[cur:], "\n")
		if strings.TrimSpace(prose) != "" {
			segs = append(segs, segment{kind: segProse, text: prose})
		}
	}
	return segs
}

// writeTextRow writes text into buf at (x0, y) with style; truncates
// at cols. Respects wide-rune widths via render/width.RuneWidth.
//
// **R2 D2 删 inline ANSI scope**: text containing \x1b[...m bytes is
// treated as raw chars (each byte → cell). spec-1.7a future ANSI
// tokenizer will parse and apply per-cell style.
func writeTextRow(buf *buffer.Grid, x0, y int, text string, s style.Style, cols int) {
	x := x0
	for i := 0; i < len(text) && x < cols; {
		r, size := utf8.DecodeRuneInString(text[i:])
		rw := width.RuneWidth(r)
		if rw <= 0 {
			i += size
			continue
		}
		if x+rw > cols {
			break
		}
		buf.SetCell(x, y, buffer.Cell{Ch: r, St: s})
		x += rw
		i += size
	}
}
