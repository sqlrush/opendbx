// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package block

import (
	"strings"
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
	"github.com/sqlrush/opendbx/internal/app/cli/render/width"
)

// gridRow reads grid row y as a string (test helper local to welcome_test).
func gridRow(t *testing.T, buf buffer.Buffer, y int) string {
	t.Helper()
	g, ok := buf.(*buffer.Grid)
	if !ok {
		t.Fatalf("buffer is not *Grid: %T", buf)
	}
	cols, rows := g.Size()
	if y < 0 || y >= rows {
		t.Fatalf("row %d out of range [0,%d)", y, rows)
	}
	var b strings.Builder
	for x := 0; x < cols; x++ {
		c := g.Cell(x, y)
		if buffer.IsContinuation(c) {
			continue // wide-rune trailing cell; the lead cell already wrote the rune
		}
		if c.Ch == 0 {
			b.WriteRune(' ')
			continue
		}
		b.WriteRune(c.Ch)
	}
	return strings.TrimRight(b.String(), " ")
}

func TestWelcome_BoxedRender(t *testing.T) {
	w := NewWelcome("v0.49.0", "~/opendbx", "Try \"为什么这条 SQL 慢\"")
	buf, err := w.Render(Context{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatalf("Render err: %v", err)
	}
	g := buf.(*buffer.Grid)
	cols, rows := g.Size()
	if rows < 4 {
		t.Fatalf("boxed welcome should have >=4 rows, got %d", rows)
	}
	top := gridRow(t, buf, 0)
	bottom := gridRow(t, buf, rows-1)
	// top has rounded corners + title; bottom has rounded corners.
	if !strings.HasPrefix(top, "╭") || !strings.Contains(top, "✻ Welcome to opendbx") {
		t.Errorf("top = %q; want ╭...✻ Welcome to opendbx...", top)
	}
	if !strings.HasPrefix(bottom, "╰") || !strings.HasSuffix(bottom, "╯") {
		t.Errorf("bottom = %q; want ╰...╯", bottom)
	}
	// body carries version, cwd, tip, and the shortcuts hint.
	all := ""
	for y := 0; y < rows; y++ {
		all += gridRow(t, buf, y) + "\n"
	}
	for _, want := range []string{"v0.49.0", "~/opendbx", "为什么这条 SQL 慢", "? for shortcuts"} {
		if !strings.Contains(all, want) {
			t.Errorf("welcome box missing %q; got:\n%s", want, all)
		}
	}
	// no row exceeds Cols (rule 20 Layer 1 invariant)
	for y := 0; y < rows; y++ {
		if w := width.Width(gridRow(t, buf, y)); w > cols {
			t.Errorf("row %d width %d exceeds cols %d", y, w, cols)
		}
	}
}

func TestWelcome_NarrowFallbackSingleLine(t *testing.T) {
	w := NewWelcome("v0.49.0", "~/opendbx", "tip")
	// Cols too small for the box → single-line graceful fallback.
	buf, err := w.Render(Context{Cols: 12, Rows: 24})
	if err != nil {
		t.Fatalf("Render err: %v", err)
	}
	g := buf.(*buffer.Grid)
	cols, rows := g.Size()
	if rows != 1 {
		t.Errorf("narrow fallback should be 1 row, got %d", rows)
	}
	row := gridRow(t, buf, 0)
	if width.Width(row) > cols {
		t.Errorf("fallback row width %d exceeds cols %d (%q)", width.Width(row), cols, row)
	}
	if !strings.Contains(row, "✻") {
		t.Errorf("fallback should still carry ✻ marker, got %q", row)
	}
}

func TestWelcome_MeasureOnly(t *testing.T) {
	w := NewWelcome("v0.49.0", "~/opendbx", "tip")
	full, _ := w.Render(Context{Cols: 80, Rows: 24})
	_, wantRows := full.(*buffer.Grid).Size()
	mo, err := w.Render(Context{Cols: 80, Rows: 24, MeasureOnly: true})
	if err != nil {
		t.Fatalf("MeasureOnly err: %v", err)
	}
	_, gotRows := mo.Size()
	if gotRows != wantRows {
		t.Errorf("MeasureOnly rows %d != full rows %d", gotRows, wantRows)
	}
}

func TestWelcome_ZeroValueDegrades(t *testing.T) {
	// Zero-value Welcome{} must not panic and must not return
	// ErrUnsupportedNode (it is a production type now, not the spec-0.13 stub).
	buf, err := Welcome{}.Render(Context{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatalf("zero-value Render err: %v", err)
	}
	if buf == nil {
		t.Fatalf("zero-value Render returned nil buffer")
	}
}

func TestWelcome_ColsZero(t *testing.T) {
	buf, err := NewWelcome("v", "c", "t").Render(Context{Cols: 0})
	if err != nil {
		t.Fatalf("Cols=0 err: %v", err)
	}
	_, rows := buf.Size()
	if rows != 0 {
		t.Errorf("Cols=0 should yield 0 rows, got %d", rows)
	}
}
