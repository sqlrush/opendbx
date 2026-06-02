// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File message.go — production Message.Render implementation (spec-1.7 D-2).
// 4-branch dispatch (Empty / 0-fence Plain / ≥1-fence Mixed) consuming
// spec-1.6 forward 4 fields + R2 D6 raw markers + R2 D11 Continued +
// 痛点 1.5 Empty. Marker tail-overflow handled symmetrically across
// renderPlainText (rowsWithMarker pre-count) and renderMixed (last-segment
// width check; spec-1.7 T-9 HIGH-1).

package block

import (
	"runtime"
	"strings"
	"unicode/utf8"

	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
	"github.com/sqlrush/opendbx/internal/app/cli/render/paint"
	"github.com/sqlrush/opendbx/internal/app/cli/render/style"
	"github.com/sqlrush/opendbx/internal/app/cli/render/width"
)

// SpeakerKind tags a Message with its conversational speaker (spec-1.25 D-5).
// The zero value SpeakerNone preserves the spec-1.7 FROZEN render (no prefix),
// so all existing block.Message{Text:...} construction sites are unaffected
// (规则 21). Speaker is a discriminator FIELD (not a separate block type)
// because user/assistant/system text share the same data shape — a string +
// wrap/markers — unlike ToolUse vs ToolResult which have genuinely different
// shapes and so are separate types (memory feedback_cc_block_design_bifurcation).
type SpeakerKind int

const (
	// SpeakerNone renders the text with no speaker decoration (default).
	SpeakerNone SpeakerKind = iota
	// SpeakerAssistant prefixes the first line with the ⏺ bullet (CC
	// AssistantTextMessage.tsx:232 BLACK_CIRCLE — per message, not per line)
	// and hanging-indents continuation lines by 2.
	SpeakerAssistant
	// SpeakerUser renders plain (no prefix), matching CC's user echo. It
	// carries no visual change vs SpeakerNone; the tag documents provenance
	// and drops the legacy "> " prefix that the caller used to prepend.
	SpeakerUser
)

// speakerBullet is the assistant speaker glyph: ⏺ on darwin, ● elsewhere
// (CC figures.ts:4 BLACK_CIRCLE platform split; spec-1.25 Q7b).
var speakerBullet = func() rune {
	if runtime.GOOS == "darwin" {
		return '⏺'
	}
	return '●'
}()

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
	// Speaker tags the conversational speaker (spec-1.25 D-5). Default
	// SpeakerNone preserves spec-1.7 behavior.
	Speaker SpeakerKind
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
	if m.Speaker == SpeakerAssistant && ctx.Cols > 2 {
		return m.renderWithBullet(ctx)
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

// renderWithBullet renders a SpeakerAssistant Message: the inner content is
// rendered at width Cols-2, then composited into a Cols-wide buffer offset
// by 2 columns, with the ⏺ bullet written at (0,0). Continuation lines fall
// under the hanging indent (their first 2 cells stay blank). Per-turn bullet
// (CC AssistantTextMessage: the glyph appears once per message, on the first
// line — not per wrapped line). Empty content renders 0 rows (no orphan
// bullet). spec-1.25 D-5.
func (m Message) renderWithBullet(ctx Context) (buffer.Buffer, error) {
	inner := m
	inner.Speaker = SpeakerNone
	innerCtx := ctx
	innerCtx.Cols = ctx.Cols - 2
	innerBuf, err := inner.Render(innerCtx)
	if err != nil {
		return nil, err
	}
	_, rows := innerBuf.Size()
	if rows == 0 {
		return measureOnlyBuf(ctx.Cols, 0), nil // no content → no bullet
	}
	if ctx.MeasureOnly {
		return measureOnlyBuf(ctx.Cols, rows), nil
	}
	out, err := buffer.NewGrid(ctx.Cols, rows)
	if err != nil {
		return measureOnlyBuf(ctx.Cols, rows), nil
	}
	if g, ok := innerBuf.(*buffer.Grid); ok {
		paint.BlitAt(out, g, 2, 0)
	}
	out.SetCell(0, 0, buffer.Cell{Ch: speakerBullet, St: themeOrDefault(ctx.Theme).Style(StyleNormal)})
	return out, nil
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
	// spec-1.7 T-9 HIGH-1: if last seg's last row is exactly cols-wide,
	// the marker "…" must occupy a new row (else it overwrites the last
	// content cell, e.g. "abcde" cols=5 + Truncated → "abcd…" wrong).
	// Symmetric with rowsWithMarker's +1 row accounting in renderPlainText.
	if (m.Truncated || m.Continued) && len(rs) > 0 && totalRows > 0 {
		lastSeg := rs[len(rs)-1]
		if grid, ok := lastSeg.buf.(*buffer.Grid); ok {
			_, srcRows := grid.Size()
			if srcRows > 0 {
				lastRowWidth := lastRowOccupiedWidth(grid, srcRows-1, ctx.Cols)
				if lastRowWidth >= ctx.Cols {
					totalRows++
				}
			}
		}
	}
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
		paint.BlitAt(stitched, grid, 0, y)
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

// lastRowOccupiedWidth scans row y of grid and returns the visual width
// (cells used by content) up to the last non-space, non-zero cell. Wide-rune
// continuation cells count as occupied. Used by renderMixed marker tail-
// overflow accounting (spec-1.7 T-9 HIGH-1).
func lastRowOccupiedWidth(grid *buffer.Grid, y, cols int) int {
	last := 0
	for x := 0; x < cols; x++ {
		c := grid.Cell(x, y)
		if c.Ch == 0 || c.Ch == ' ' {
			continue
		}
		// Either main char or continuation cell counts as occupied.
		last = x + 1
	}
	return last
}
