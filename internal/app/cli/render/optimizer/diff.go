// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package optimizer

import (
	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
)

// DiffEngine computes minimal Patch slices between two Buffer snapshots.
//
// spec-1.3 design: naive cell-by-cell row-major scan + buffer.IsContinuation
// skip (spec-1.2 R2 D1 SSOT consumer contract) + PatchResize on size
// mismatch + nil-prev fullRedraw fast path. No coalescing, no cursor-move
// optimisation, no SGR delta — those are reserved for spec-1.3a (the
// PatchMoveCursor / PatchStyleChange kinds are never emitted here).
//
// Concurrency: DiffEngine is a zero-state struct ({}); *DiffEngine is
// safe to share across goroutines (Q8 ★A). Input Buffers remain caller-
// owned for the lifetime of Diff per the spec-1.2 D3 ownership transfer
// contract.
type DiffEngine struct{}

// NewDiffEngine constructs a fresh DiffEngine.
func NewDiffEngine() *DiffEngine { return &DiffEngine{} }

// Compile-time assertion that *DiffEngine satisfies Optimizer.
var _ Optimizer = (*DiffEngine)(nil)

// Diff scans next vs prev row-major and emits the minimal Patch slice.
//
// Precondition (spec-1.3 R-10, R2 codex R1+R3 MED-1): next MUST be
// non-nil. nil next is a caller lifecycle bug — the scheduler frame
// loop always holds an Acquired buffer. We do NOT add a no-op fallback
// branch (would mask bugs); Go's natural nil-deref panic on next.Size()
// is the explicit contract violation signal.
//
// Boundary paths:
//
//   - prev == nil OR resized → fullRedraw path. Scan emits PatchSetCell
//     ONLY for non-zero cells. The G1 clean-surface precondition
//     (spec-1.3 R2 user hard guard) says prev==nil means the Driver has
//     already established a blank surface via Init/Clear/Resize; the
//     scheduler (spec-1.4) is responsible for invoking Driver.Init
//     before the first Diff. Emitting PatchSetCell{Cell{}} for already-
//     blank cells would be redundant noise.
//
//   - prev.Size() != next.Size() → emit PatchResize{NewCols, NewRows}
//     as patch[0], then fullRedraw the next buffer. The Driver adapter
//     (spec-1.4) is responsible for clearing the new viewport when
//     applying PatchResize.
//
//   - normal diff (prev != nil && !resized) → cell-by-cell value
//     compare (Cell is value-comparable per spec-0.13 D-3). On mismatch
//     emit PatchSetCell{Cell: nextCell} — INCLUDING when nextCell is
//     Cell{} but prevCell is not (the "clear" patch). Driver.SetCell
//     semantics for Ch=0 → clear-to-blank are deferred to the spec-1.4
//     Driver adapter contract; this layer only produces patch data.
//
//   - continuation cells (buffer.IsContinuation, spec-1.2 R2 D1 SSOT)
//     are skipped — the main cell at (x-1, y) has already been emitted
//     and Driver.SetCell handles the wide-rune cursor advance.
func (e *DiffEngine) Diff(prev, next buffer.Buffer) []Patch {
	nc, nr := next.Size() // panics if next == nil (caller bug per godoc)
	// Pre-size the patch slice. Modest hint for typical low-churn frames;
	// append's geometric growth handles full-redraw spikes (R-5: total
	// copy is amortized O(n) not O(n²)).
	patches := make([]Patch, 0, 64)

	resized := false
	if prev != nil {
		pc, pr := prev.Size()
		if pc != nc || pr != nr {
			patches = append(patches, Patch{
				Kind:    PatchResize,
				NewCols: nc,
				NewRows: nr,
			})
			resized = true
		}
	}
	fullRedraw := prev == nil || resized

	for y := 0; y < nr; y++ {
		for x := 0; x < nc; x++ {
			nCell := next.Cell(x, y)
			if buffer.IsContinuation(nCell) {
				continue // main cell at (x-1, y) already emitted
			}
			if fullRedraw {
				// G1 clean-surface precondition: Driver has already
				// established a blank surface; emit only non-zero cells.
				if nCell == (buffer.Cell{}) {
					continue
				}
			} else {
				// normal diff: cell-by-cell value compare. Cell{} on
				// next when prev is non-zero IS a meaningful clear
				// patch — Driver.SetCell(x,y,0,zero) clears that cell.
				if prev.Cell(x, y) == nCell {
					continue
				}
			}
			patches = append(patches, Patch{
				Kind: PatchSetCell,
				X:    x,
				Y:    y,
				Cell: nCell,
			})
		}
	}
	return patches
}
