// Package badpkg is a test fixture for paint-pattern-lint PAINT-1.
// Each function in this file demonstrates a violation that the lint
// MUST catch (cross-receiver bare cell-to-cell copies missing the
// buffer.IsContinuation guard).
package badpkg

import "example.com/paint-pattern-lint-fixtures/buffer"

// BadDirectInline is the direct call pattern — dst.SetCell(x, y, src.Cell(...))
// inline argument, cross-receiver, no guard.
func BadDirectInline(dst *buffer.Grid, src buffer.Buffer) {
	dst.SetCell(0, 0, src.Cell(0, 0))
}

// BadOneHopUnguarded is the one-hop assign pattern — `c := src.Cell(...);
// dst.SetCell(..., c)` with no IsContinuation guard between assignment
// and write. The 5/28 user-evidence bug class lived here.
func BadOneHopUnguarded(dst *buffer.Grid, src buffer.Buffer) {
	c := src.Cell(0, 0)
	dst.SetCell(0, 0, c)
}

// BadOneHopGuardDoesNotExit checks the negative case for the
// IsContinuation guard: the if-body must contain a flow-exit
// (continue/break/return). Without an exit the taint is NOT cleared.
func BadOneHopGuardDoesNotExit(dst *buffer.Grid, src buffer.Buffer) {
	c := src.Cell(0, 0)
	if buffer.IsContinuation(c) {
		_ = c // no continue/break/return
	}
	dst.SetCell(0, 0, c)
}
