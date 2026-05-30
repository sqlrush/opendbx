// Package goodpkg is a test fixture for paint-pattern-lint — each
// function exercises a path that the lint must NOT report (same-receiver
// exempt, guarded one-hop, exempt directive, paint helper usage).
package goodpkg

import "example.com/paint-pattern-lint-fixtures/buffer"

// SameReceiverIsExempt is the walker-style read/modify/write on a single
// grid — exempt because dst==src, no cross-buffer continuation risk.
func SameReceiverIsExempt(g *buffer.Grid) {
	c := g.Cell(0, 0)
	g.SetCell(0, 0, c)
}

// SameReceiverDirectIsExempt mirrors the inline-call form of the
// same-receiver pattern.
func SameReceiverDirectIsExempt(g *buffer.Grid) {
	g.SetCell(0, 0, g.Cell(0, 0))
}

// GuardedOneHopOK shows the canonical continuation-skip pattern that
// matches the writeRawCells contract — once buffer.IsContinuation fires
// `continue`, the tainted variable is cleared and the subsequent
// dst.SetCell is safe (the continuation is auto-written by the wide
// main's SetCell call elsewhere).
func GuardedOneHopOK(dst *buffer.Grid, cells []buffer.Cell) {
	for i, c := range cells {
		_ = i
		if buffer.IsContinuation(c) {
			continue
		}
		dst.SetCell(0, 0, c)
	}
}

// GuardedOneHopReturnOK is the same shape but with an early return as
// the exit token — also clears taint.
func GuardedOneHopReturnOK(dst *buffer.Grid, src buffer.Buffer) {
	c := src.Cell(0, 0)
	if buffer.IsContinuation(c) {
		return
	}
	dst.SetCell(0, 0, c)
}

// ExemptDirective shows the suppress-comment escape hatch on a line
// directly above the offending call. spec-X.Y reference is required by
// hint output but the lint only matches the literal directive.
func ExemptDirective(dst *buffer.Grid, src buffer.Buffer) {
	// paint-pattern-lint:exempt -- spec-1.20.2 D-3: fixture demonstrates suppress directive
	dst.SetCell(0, 0, src.Cell(0, 0))
}

// LiteralCellOK constructs a Cell{} literal — not from src.Cell(),
// hence no taint, no violation. Production code uses this pattern in
// block/code.go and block/message.go.
func LiteralCellOK(dst *buffer.Grid) {
	dst.SetCell(0, 0, buffer.Cell{Ch: 'A'})
}
