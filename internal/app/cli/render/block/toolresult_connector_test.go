// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package block

import (
	"strings"
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
)

// TestToolResult_Connector: spec-1.25 D-6 renders the result under a "  ⎿ "
// tree connector with content hanging-indented by 4 columns.
func TestToolResult_Connector(t *testing.T) {
	tr := NewToolResult("tu1", "Bash", "done", false)
	buf, err := tr.Render(Context{Cols: 60, Rows: 24})
	if err != nil {
		t.Fatalf("render err: %v", err)
	}
	g := buf.(*buffer.Grid)
	_, rows := g.Size()
	if rows == 0 {
		t.Fatal("expected >=1 row")
	}
	// connector rune ⎿ sits at column 2 of row 0
	if got := g.Cell(2, 0).Ch; got != toolResultConnector {
		t.Errorf("col2 row0 = %q; want connector %q", got, toolResultConnector)
	}
	// columns 0,1 are blank gutter
	for x := 0; x < 2; x++ {
		if c := g.Cell(x, 0).Ch; c != 0 && c != ' ' {
			t.Errorf("gutter col%d = %q; want blank", x, c)
		}
	}
	// content (indicator + text) begins at column 4
	row0 := gridRow(t, buf, 0)
	if !strings.Contains(row0, "done") {
		t.Errorf("row0 = %q; want result text 'done' after connector", row0)
	}
	idx := strings.IndexRune(row0, '⎿')
	if idx < 0 || !strings.Contains(row0[idx:], "done") {
		t.Errorf("row0 = %q; want ⎿ before content", row0)
	}
}

// TestToolResult_ConnectorNarrowFallback: when cols <= connector width the
// result renders flush-left (no connector) and never panics / overflows.
func TestToolResult_ConnectorNarrowFallback(t *testing.T) {
	tr := NewToolResult("tu1", "Bash", "x", false)
	buf, err := tr.Render(Context{Cols: 3, Rows: 24})
	if err != nil {
		t.Fatalf("render err: %v", err)
	}
	g := buf.(*buffer.Grid)
	cols, rows := g.Size()
	for y := 0; y < rows; y++ {
		// no row exceeds cols
		w := 0
		for x := 0; x < cols; x++ {
			if !buffer.IsContinuation(g.Cell(x, y)) && g.Cell(x, y).Ch != 0 {
				w = x + 1
			}
		}
		if w > cols {
			t.Errorf("row %d width %d exceeds cols %d", y, w, cols)
		}
	}
}
