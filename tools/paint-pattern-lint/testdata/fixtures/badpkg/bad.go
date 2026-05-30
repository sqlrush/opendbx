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

// BadInterfaceDestination proves interface-typed destinations are sinks too;
// production helpers often accept buffer.Buffer rather than *buffer.Grid.
func BadInterfaceDestination(dst buffer.Buffer, src buffer.Buffer) {
	dst.SetCell(0, 0, src.Cell(0, 0))
}

// BadOneHopVarDecl catches `var c = src.Cell(...)`, not just short assigns.
func BadOneHopVarDecl(dst *buffer.Grid, src buffer.Buffer) {
	var c = src.Cell(0, 0)
	dst.SetCell(0, 0, c)
}

// BadOneHopMultiAssign catches multi-LHS assignment with a tainted RHS.
func BadOneHopMultiAssign(dst *buffer.Grid, src buffer.Buffer) {
	_, c := 1, src.Cell(0, 0)
	dst.SetCell(0, 0, c)
}

// BadOneHopAlias catches simple aliases of a tainted cell.
func BadOneHopAlias(dst *buffer.Grid, src buffer.Buffer) {
	c := src.Cell(0, 0)
	alias := c
	dst.SetCell(0, 0, alias)
}

// BadNestedGuardDoesNotDominate catches a guard nested under an unrelated
// branch; it does not dominate the later SetCell and must not clear taint.
func BadNestedGuardDoesNotDominate(dst *buffer.Grid, src buffer.Buffer, flag bool) {
	c := src.Cell(0, 0)
	if flag {
		if buffer.IsContinuation(c) {
			return
		}
	}
	dst.SetCell(0, 0, c)
}
