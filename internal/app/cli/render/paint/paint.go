// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package paint

import (
	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
)

// Blit copies a top-left (cols × rows) rect from src into dst at (0, 0),
// honoring the spec-1.7 T-9 HIGH-2 continuation contract: src cells
// flagged buffer.IsContinuation are skipped, because dst.SetCell on a
// wide-main auto-writes its own continuation; writing the continuation
// again would trigger clearWideOverlap and erase the wide-main at (x-1).
//
// Contract:
//   - nil dst or nil src → no-op.
//   - cols ≤ 0 or rows ≤ 0 → no-op.
//   - cols / rows are HARD bounds (caller-supplied clip — preserves the
//     spec-1.20.1 scrollback/cache.go copyCells contract where callers
//     pass measured dimensions independent of src.Size()).
//   - Cells with destination coordinates outside dst silently drop.
//
// Use BlitAt when the destination origin is non-zero.
func Blit(dst *buffer.Grid, src buffer.Buffer, cols, rows int) {
	if dst == nil || src == nil || cols <= 0 || rows <= 0 {
		return
	}
	dstCols, dstRows := dst.Size()
	srcCols, srcRows := src.Size()
	// Take the intersection of caller-supplied clip and src extent.
	maxX := cols
	if srcCols < maxX {
		maxX = srcCols
	}
	if dstCols < maxX {
		maxX = dstCols
	}
	maxY := rows
	if srcRows < maxY {
		maxY = srcRows
	}
	if dstRows < maxY {
		maxY = dstRows
	}
	for y := 0; y < maxY; y++ {
		for x := 0; x < maxX; x++ {
			c := src.Cell(x, y)
			if buffer.IsContinuation(c) {
				continue
			}
			dst.SetCell(x, y, c)
		}
	}
}

// BlitAt copies the entire src buffer into dst at offset (xOff, yOff),
// honoring the same continuation contract. Negative offsets and cells
// outside dst silently crop, matching the existing paintBufferAt
// behavior in spec-1.20.1's five callsites.
//
//   - nil dst or nil src → no-op.
//   - dst cells outside [0, dstCols) × [0, dstRows) silently drop.
//   - Continuation cells from src are skipped; SetCell auto-writes the
//     continuation for the corresponding wide-main.
func BlitAt(dst *buffer.Grid, src buffer.Buffer, xOff, yOff int) {
	if dst == nil || src == nil {
		return
	}
	dstCols, dstRows := dst.Size()
	srcCols, srcRows := src.Size()
	for sy := 0; sy < srcRows; sy++ {
		dy := yOff + sy
		if dy < 0 || dy >= dstRows {
			continue
		}
		for sx := 0; sx < srcCols; sx++ {
			dx := xOff + sx
			if dx < 0 || dx >= dstCols {
				continue
			}
			c := src.Cell(sx, sy)
			if buffer.IsContinuation(c) {
				continue
			}
			dst.SetCell(dx, dy, c)
		}
	}
}
