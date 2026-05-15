// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

//go:build !windows

package uitest

// vt10x cell-grid model (verified 2026-05-15 against
// github.com/hinshun/vt10x v0.0.0-20220301184237):
//
//   - Each Cell(x, y) holds exactly one rune (Glyph.Char).
//   - Wide CJK characters advance the cursor by 1, not 2 — vt10x does
//     NOT model wcwidth. Display width on a real terminal will differ
//     from logical cell width.
//   - Unwritten cells return Glyph{Char: ' '} (regular U+0020 space).
//
// Implication for golden tests:
//
// CellGrid / CellGridRunes return one rune per cell (Cols * Rows total).
// Display alignment in real terminals may differ for CJK content;
// that's covered by spec-0.11.5 visual diff layer (freeze + pixelmatch),
// not by this cell-level harness.
//
// Earlier spec drafts (R4) assumed vt10x produced continuation cells
// with Char==0 — that assumption was wrong; T-6 implementation
// dogfood corrected the model. See T-13 errata for spec patch.

// CellGrid returns one string per terminal row. Each rune from vt10x is
// emitted at its logical cell position.
//
// Concurrency: takes the vt10x.Terminal Lock for the duration of the
// read, returning a consistent snapshot. Do not retain a returned row
// across PTY writes — copy if needed.
func (t *Terminal) CellGrid() []string {
	rows := t.CellGridRunes()
	out := make([]string, len(rows))
	for i, row := range rows {
		out[i] = string(row)
	}
	return out
}

// CellGridRunes returns one []rune per row, length == cols. Index by
// column: out[y][x].
func (t *Terminal) CellGridRunes() [][]rune {
	t.vt.Lock()
	defer t.vt.Unlock()
	cols, rows := t.vt.Size()
	out := make([][]rune, rows)
	for y := 0; y < rows; y++ {
		row := make([]rune, cols)
		for x := 0; x < cols; x++ {
			row[x] = t.vt.Cell(x, y).Char
		}
		out[y] = row
	}
	return out
}
