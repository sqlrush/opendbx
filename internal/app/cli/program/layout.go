// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package program

// Layout describes the current terminal geometry and the three-zone
// split: scrollback occupies rows [0, Rows-2), the input row sits at
// Rows-2, the status line at Rows-1.
//
// spec-1.16/1.17 will extend layout to multi-row input + side panels
// via LayoutPlugin; baseline 3 zones are hardcoded for spec-1.15.
type Layout struct {
	Cols, Rows int
}

// ScrollbackSize returns the cols × rows available for Model.View
// rendering — the entire terminal width with the bottom two rows
// reserved for input + status. When the terminal is too small to fit
// all three zones (Rows < 3) ScrollbackSize collapses gracefully.
func (l Layout) ScrollbackSize() (cols, rows int) {
	cols = l.Cols
	rows = l.Rows - 2
	if rows < 0 {
		rows = 0
	}
	return
}

// InputRow returns the row index of the input row (Rows-2). Returns
// -1 when the terminal is too small (Rows < 2).
func (l Layout) InputRow() int {
	if l.Rows < 2 {
		return -1
	}
	return l.Rows - 2
}

// StatusLine returns the row index of the status line (Rows-1).
// Returns -1 when the terminal has zero rows.
func (l Layout) StatusLine() int {
	if l.Rows < 1 {
		return -1
	}
	return l.Rows - 1
}

// LayoutPlugin is the forward-link extension point referenced by D-4
// for spec-1.16 (multi-row input) and spec-1.17 (side panels). spec-
// 1.15 ships only the interface signature; the baseline Program uses
// the hardcoded 3-zone split above.
type LayoutPlugin interface {
	Regions(l Layout) []Region
}

// Region is one rectangular sub-area of the screen with a stable name.
type Region struct {
	Name   string
	Top    int
	Bottom int
}
