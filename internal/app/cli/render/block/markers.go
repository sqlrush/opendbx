// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File markers.go — Truncated / Continued / Empty marker helpers + the
// measureOnlyBuf abstraction. spec-1.7 D-5 + R2 D1 (CRIT-1: Continued
// "…" + StyleDimmed; Truncated "…" + StyleWarning; same rune,
// distinguished by Style per spec-1.6 FROZEN godoc).
//
// **Empty placeholder text** "(no output)" + StyleDimmed (spec-1.7 Q6
// ★A; T-9 R3 fixture verify may errata).

package block

import (
	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
	"github.com/sqlrush/opendbx/internal/app/cli/render/style"
	"github.com/sqlrush/opendbx/internal/app/cli/render/width"
)

const (
	// markerRune is the visual marker rune used by both Truncated and
	// Continued paths. Style (StyleWarning vs StyleDimmed) distinguishes
	// them per spec-1.6 FROZEN godoc + spec-1.7 R2 D1 (CRIT-1).
	markerRune = '…'
	// emptyPlaceholderText is shown when Message.Empty=true (痛点 1.5).
	emptyPlaceholderText = "(no output)"
)

// applyTruncated appends "…" + StyleWarning to the last row of buf.
// If the last row's content already fills cols, the marker is written
// over the last cell (overwrite); MeasureOnly callers must account for
// possible tail-row overflow via rowsWithMarker.
func applyTruncated(buf *buffer.Grid, theme StyleTheme) {
	applyTailMarker(buf, themeOrDefault(theme).Style(StyleWarning))
}

// applyContinued appends "…" + StyleDimmed to the last row of buf.
// Same rune as Truncated; Style distinguishes per spec-1.6 FROZEN
// godoc + spec-1.7 R2 D1.
func applyContinued(buf *buffer.Grid, theme StyleTheme) {
	applyTailMarker(buf, themeOrDefault(theme).Style(StyleDimmed))
}

// applyTailMarker writes markerRune at the last cell of the last row.
// If that cell is already non-blank, overwrites it. Caller's
// rowsWithMarker decides whether to allocate +1 row for tail overflow.
func applyTailMarker(buf *buffer.Grid, s style.Style) {
	if buf == nil {
		return
	}
	cols, rows := buf.Size()
	if cols <= 0 || rows <= 0 {
		return
	}
	// Find last non-blank cell in last row to overwrite if at edge,
	// else write at the first blank cell.
	y := rows - 1
	for x := cols - 1; x >= 0; x-- {
		c := buf.Cell(x, y)
		if c.Ch != 0 && c.Ch != ' ' {
			// Append marker after this char (if room), else overwrite.
			if x+1 < cols {
				buf.SetCell(x+1, y, buffer.Cell{Ch: markerRune, St: s})
			} else {
				buf.SetCell(x, y, buffer.Cell{Ch: markerRune, St: s})
			}
			return
		}
	}
	// All-blank last row: write marker at column 0.
	buf.SetCell(0, y, buffer.Cell{Ch: markerRune, St: s})
}

// emptyPlaceholder returns a Buffer rendering "(no output)" at row 0
// + StyleDimmed (痛点 1.5; spec-1.7 Q6 ★A). MeasureOnly returns 1-row
// measureOnlyBuf without cell writes.
func emptyPlaceholder(ctx Context) buffer.Buffer {
	theme := themeOrDefault(ctx.Theme)
	if ctx.MeasureOnly {
		return measureOnlyBuf(ctx.Cols, 1)
	}
	if ctx.Cols <= 0 {
		return measureOnlyBuf(0, 1)
	}
	buf, err := buffer.NewGrid(ctx.Cols, 1)
	if err != nil {
		return measureOnlyBuf(ctx.Cols, 1)
	}
	s := theme.Style(StyleDimmed)
	for x, r := range []rune(emptyPlaceholderText) {
		if x >= ctx.Cols {
			break
		}
		buf.SetCell(x, 0, buffer.Cell{Ch: r, St: s})
	}
	return buf
}

// rowsWithMarker computes the row count for content + optional marker.
// **R2 D6 HIGH-E**: MUST invoke wrap() to count soft-wrap lines correctly;
// strings.Count("\n") would miss wrap rows → spec-1.5 sb offset drift.
//
// Marker tail-overflow: if last wrapped line's visual width >= cols and
// either Truncated or Continued is set, +1 row to accommodate marker.
func rowsWithMarker(text string, cols int, policy WrapPolicy, m Message) int {
	if cols <= 0 {
		return 0
	}
	lines := wrap(text, cols, policy)
	rows := len(lines)
	if rows == 0 {
		return 0
	}
	if m.Truncated || m.Continued {
		lastW := width.Width(lines[len(lines)-1])
		if lastW >= cols {
			rows++
		}
	}
	return rows
}

// measureOnlyBuf is a Buffer implementation that reports Size() but
// no-ops cell I/O. Used by MeasureOnly fast path. Distinct from
// buffer.NewGrid which rejects rows ≤ 0 (spec-1.7 R2 D6 HIGH-E: required
// for Empty-text 0-row contract).
type measureOnlyBufImpl struct {
	cols, rows int
}

func measureOnlyBuf(cols, rows int) buffer.Buffer {
	if cols < 0 {
		cols = 0
	}
	if rows < 0 {
		rows = 0
	}
	return &measureOnlyBufImpl{cols: cols, rows: rows}
}

// Cell returns the zero Cell. spec-1.2 D-3 single-owner contract: this
// buffer is owned by the caller stack; never escapes to scrollback.
func (b *measureOnlyBufImpl) Cell(x, y int) buffer.Cell {
	return buffer.Cell{}
}

// SetCell is a no-op (MeasureOnly contract).
func (b *measureOnlyBufImpl) SetCell(x, y int, c buffer.Cell) {}

// Size returns the reported dimensions.
func (b *measureOnlyBufImpl) Size() (cols, rows int) {
	return b.cols, b.rows
}

// Resize updates the reported dimensions (no-op for actual content).
func (b *measureOnlyBufImpl) Resize(cols, rows int) {
	if cols < 0 {
		cols = 0
	}
	if rows < 0 {
		rows = 0
	}
	b.cols = cols
	b.rows = rows
}
