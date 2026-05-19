// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package buffer

// WideContinuation is the sentinel rune marking a cell that is the
// 2nd half of an East Asian wide rune (RuneWidth == 2). The main
// cell at (x, y) holds the actual rune; the continuation cell at
// (x+1, y) holds Cell{Ch: WideContinuation, St: <inherited from main>}.
//
// Rationale (spec-1.2 R2 D1 ★A2): rune = -1 is not a valid Unicode
// code point, so it cannot collide with any user-supplied rune
// (including NUL '\x00' and the ZWSP U+200B). Empty cells (Cell{}
// with Ch == 0) are distinct from continuation cells; consumers MUST
// use the IsContinuation predicate rather than hard-coding Ch == 0
// or Ch == -1 comparisons.
//
// Callers (spec-1.3 ANSI optimizer, spec-1.7 block composer):
//   - on encountering a continuation cell while scanning a buffer,
//     emit nothing (the terminal cursor has already advanced past
//     this column by virtue of the wide rune's main cell);
//   - empty Cell{} is a different semantic — emit a space (opaque
//     background) or no-op (transparent overlay) per the composer
//     strategy.
//
// spec-0.13 § 11.6 errata pins this contract for the spec-0.13
// Buffer interface (no signature change).
const WideContinuation rune = -1

// IsContinuation reports whether c is the 2nd half of a wide rune
// (i.e., its Ch equals WideContinuation). Empty cells (Cell{} with
// Ch == 0) and any cell carrying a real rune (narrow or wide-main)
// return false.
//
// This is the only API consumers should use to detect continuation
// cells; hard-coding `cell.Ch == buffer.WideContinuation` couples
// callers to an implementation detail.
func IsContinuation(c Cell) bool {
	return c.Ch == WideContinuation
}
