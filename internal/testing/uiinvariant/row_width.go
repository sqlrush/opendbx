// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package uiinvariant

import (
	"testing"

	"github.com/mattn/go-runewidth"
)

// CheckRowWidth verifies every row of grid fits within cols cells.
// Uses runewidth with EastAsianWidth=false per CLAUDE.md § 3.9
// invariant. Fails fast with the first violating row.
//
// spec-0.11.5 D-1: spec-0.10 IMP-8 import exception lets uiinvariant
// import runewidth directly. spec-1.14 render/width.Width() landing
// will route through that wrapper; uiinvariant migrates then.
func CheckRowWidth(t testing.TB, grid []string, cols int) {
	t.Helper()
	for y, row := range grid {
		w := runewidth.StringWidth(row)
		if w > cols {
			t.Fatalf("CheckRowWidth: row %d width %d > cols %d (text: %q)",
				y, w, cols, row)
			return
		}
	}
}
