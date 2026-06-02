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
	t.Parallel()
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

// TestToolResult_ConnectorWrapHangingIndent: a result long enough to wrap
// hangs continuation rows under the connector — content at column 4, and the
// ⎿ glyph appears ONLY on the first row (spec-1.25 D-6 hanging-indent).
func TestToolResult_ConnectorWrapHangingIndent(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("word ", 20) // forces multiple wrapped rows at narrow cols
	tr := NewToolResult("tu1", "Bash", long, false)
	buf, err := tr.Render(Context{Cols: 20, Rows: 24, Wrap: WrapSoft})
	if err != nil {
		t.Fatalf("render err: %v", err)
	}
	g := buf.(*buffer.Grid)
	_, rows := g.Size()
	if rows < 2 {
		t.Fatalf("expected wrapped result (>=2 rows), got %d", rows)
	}
	// row 0 carries the connector at col 2
	if g.Cell(2, 0).Ch != toolResultConnector {
		t.Errorf("row0 col2 = %q; want connector", g.Cell(2, 0).Ch)
	}
	// continuation rows: NO connector anywhere, and content starts at col 4
	for y := 1; y < rows; y++ {
		row := gridRow(t, buf, y)
		if strings.ContainsRune(row, toolResultConnector) {
			t.Errorf("continuation row %d has a connector glyph: %q", y, row)
		}
		// first 4 columns are the hanging-indent gutter (blank)
		for x := 0; x < 4; x++ {
			if c := g.Cell(x, y).Ch; c != 0 && c != ' ' {
				t.Errorf("row %d col %d = %q; want blank gutter (hanging indent)", y, x, c)
			}
		}
	}
}

// TestToolResult_ConnectorNarrowFallback: when cols <= connector width the
// result renders flush-left (no connector) and never panics / overflows.
func TestToolResult_ConnectorNarrowFallback(t *testing.T) {
	t.Parallel()
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
