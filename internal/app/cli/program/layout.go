// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package program

// inputBoxMinRows is the spec-1.25 §4.1 N=5 correctness floor for the
// 3-row top/bottom-rule input box: status(1) + box(3) + >=1 scrollback
// row. Below it the input degrades to the spec-1.16 single-row form so
// ScrollbackSize never goes negative (R-1 graceful).
const inputBoxMinRows = 5

// Layout describes the current terminal geometry. spec-1.25 D-3: when
// Rows >= inputBoxMinRows the input is a 3-row top/bottom-rule box (CC
// PromptInput.tsx:2268 borderLeft/Right=false parity) occupying rows
// [Rows-4, Rows-1): top rule (Rows-4) / content (Rows-3) / bottom rule
// (Rows-2); the status line sits at Rows-1 and scrollback at [0, Rows-4).
// Below the floor it degrades to the spec-1.15/1.16 single-row split
// (input Rows-2, status Rows-1, scrollback [0, Rows-2)).
//
// LayoutPlugin (below) remains an unwired forward-link seam (spec-1.17
// side panels); spec-1.25 deliberately uses the concrete accessors here
// rather than the plugin (R2 CRIT-B: plugin has no consumer).
type Layout struct {
	Cols, Rows int
}

// BoxMode reports whether the terminal is tall enough for the 3-row
// input box (Rows >= inputBoxMinRows). Callers paint the single-row
// input when false.
func (l Layout) BoxMode() bool {
	return l.Rows >= inputBoxMinRows
}

// ScrollbackSize returns the cols × rows available for Model.View
// rendering: full width with the bottom zones reserved (4 rows in box
// mode: top/content/bottom rule + status; 2 rows in single-row mode).
// Collapses gracefully (never negative) on tiny terminals.
func (l Layout) ScrollbackSize() (cols, rows int) {
	cols = l.Cols
	if l.BoxMode() {
		rows = l.Rows - 4
	} else {
		rows = l.Rows - 2
	}
	if rows < 0 {
		rows = 0
	}
	return
}

// InputRow returns the row index of the input CONTENT row (where the
// buffer + cursor are painted) in both modes: Rows-3 in box mode,
// Rows-2 in single-row mode. Returns -1 when too small (Rows < 2).
func (l Layout) InputRow() int {
	if l.BoxMode() {
		return l.Rows - 3
	}
	if l.Rows < 2 {
		return -1
	}
	return l.Rows - 2
}

// InputBoxTop returns the top-rule row (Rows-4) in box mode, else -1.
func (l Layout) InputBoxTop() int {
	if !l.BoxMode() {
		return -1
	}
	return l.Rows - 4
}

// InputBoxBottom returns the bottom-rule row (Rows-2) in box mode,
// else -1.
func (l Layout) InputBoxBottom() int {
	if !l.BoxMode() {
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
