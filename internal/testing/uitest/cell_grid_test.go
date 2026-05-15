// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

//go:build !windows

package uitest

import (
	"strings"
	"testing"
	"time"
)

func TestCellGrid_AsciiBanner(t *testing.T) {
	term := Term(t, helperCmd(t, "banner"), 20, 5)
	term.WaitFor(t, func(*Terminal) bool {
		return strings.Contains(strings.Join(term.CellGrid(), "\n"), "ready")
	}, time.Second)
	rows := term.CellGrid()
	if len(rows) != 5 {
		t.Fatalf("rows=%d want 5", len(rows))
	}
	if !strings.HasPrefix(rows[0], "> ready") {
		t.Errorf("row 0 prefix wrong: %q", rows[0])
	}
}

func TestCellGrid_WideRune(t *testing.T) {
	term := Term(t, helperCmd(t, "wide"), 20, 3)
	term.WaitFor(t, func(*Terminal) bool {
		return strings.Contains(strings.Join(term.CellGrid(), "\n"), "中")
	}, time.Second)
	runes := term.CellGridRunes()
	// vt10x model: each rune occupies exactly one column.
	// "中文 hello" should fill columns 0-9 with: 中,文,space,h,e,l,l,o.
	want := []rune{'中', '文', ' ', 'h', 'e', 'l', 'l', 'o'}
	for i, w := range want {
		if runes[0][i] != w {
			t.Errorf("col %d = %q (%U), want %q (%U)", i, runes[0][i], runes[0][i], w, w)
		}
	}
}

func TestCellGrid_UnwrittenCellIsSpace(t *testing.T) {
	term := Term(t, helperCmd(t, "banner"), 80, 24)
	term.WaitFor(t, func(*Terminal) bool {
		return strings.Contains(strings.Join(term.CellGrid(), "\n"), "ready")
	}, time.Second)
	runes := term.CellGridRunes()
	// vt10x initializes unwritten cells to ' ' (U+0020).
	if runes[10][50] != ' ' {
		t.Errorf("unwritten cell = %q (%U), want ' '", runes[10][50], runes[10][50])
	}
}
