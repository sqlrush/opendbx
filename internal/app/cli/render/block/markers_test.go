// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package block

import (
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
)

// Marker helpers tests per spec-1.7 § 4.2 markers_test 8+ cases.

func TestApplyTruncated_StyleWarning(t *testing.T) {
	buf, _ := buffer.NewGrid(10, 1)
	buf.SetCell(0, 0, buffer.Cell{Ch: 'a'})
	buf.SetCell(1, 0, buffer.Cell{Ch: 'b'})
	buf.SetCell(2, 0, buffer.Cell{Ch: 'c'})
	applyTruncated(buf, DefaultTheme{})
	c := buf.Cell(3, 0)
	if c.Ch != '…' {
		t.Errorf("Cell(3,0).Ch=%q, want …", c.Ch)
	}
	want := DefaultTheme{}.Style(StyleWarning)
	if c.St != want {
		t.Errorf("Style mismatch: %+v vs %+v", c.St, want)
	}
}

func TestApplyContinued_StyleDimmed(t *testing.T) {
	buf, _ := buffer.NewGrid(10, 1)
	buf.SetCell(0, 0, buffer.Cell{Ch: 'a'})
	buf.SetCell(1, 0, buffer.Cell{Ch: 'b'})
	buf.SetCell(2, 0, buffer.Cell{Ch: 'c'})
	applyContinued(buf, DefaultTheme{})
	c := buf.Cell(3, 0)
	if c.Ch != '…' {
		t.Errorf("Cell(3,0).Ch=%q, want …", c.Ch)
	}
	want := DefaultTheme{}.Style(StyleDimmed)
	if c.St != want {
		t.Errorf("Style mismatch: %+v vs %+v", c.St, want)
	}
}

func TestApplyTruncated_NilBuffer_NoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic: %v", r)
		}
	}()
	applyTruncated(nil, DefaultTheme{})
}

func TestApplyTruncated_TailFullRowOverwrite(t *testing.T) {
	// Cell at the very last position; marker should overwrite (no extra row in this helper).
	buf, _ := buffer.NewGrid(3, 1)
	buf.SetCell(0, 0, buffer.Cell{Ch: 'a'})
	buf.SetCell(1, 0, buffer.Cell{Ch: 'b'})
	buf.SetCell(2, 0, buffer.Cell{Ch: 'c'})
	applyTruncated(buf, DefaultTheme{})
	c := buf.Cell(2, 0)
	if c.Ch != '…' {
		t.Errorf("Last cell should be overwritten with …, got %q", c.Ch)
	}
}

func TestEmptyPlaceholder_NoOutput(t *testing.T) {
	ctx := Context{Cols: 20, Theme: DefaultTheme{}}
	buf := emptyPlaceholder(ctx)
	_, rows := buf.Size()
	if rows != 1 {
		t.Fatalf("rows=%d, want 1", rows)
	}
	// First few chars match "(no output)".
	wantText := emptyPlaceholderText
	for i, r := range []rune(wantText) {
		c := buf.Cell(i, 0)
		if c.Ch != r {
			t.Errorf("Cell(%d,0).Ch=%q, want %q", i, c.Ch, r)
		}
	}
}

func TestEmptyPlaceholder_MeasureOnly(t *testing.T) {
	ctx := Context{Cols: 20, MeasureOnly: true, Theme: DefaultTheme{}}
	buf := emptyPlaceholder(ctx)
	_, rows := buf.Size()
	if rows != 1 {
		t.Fatalf("MeasureOnly rows=%d, want 1", rows)
	}
	if buf.Cell(0, 0).Ch != 0 {
		t.Error("MeasureOnly should not write cells")
	}
}

func TestRowsWithMarker_TailOverflow(t *testing.T) {
	// Text fits exactly cols → marker should push to next row.
	rows := rowsWithMarker("abcde", 5, WrapSoft, Message{Truncated: true})
	if rows != 2 {
		t.Errorf("tail-full + Truncated: rows=%d, want 2 (marker overflow)", rows)
	}
}

func TestRowsWithMarker_InvokeWrap(t *testing.T) {
	// Soft wrap: 20 chars at cols=5 should wrap to 4 rows.
	text := "aaaa bbbb cccc dddd"
	rows := rowsWithMarker(text, 5, WrapSoft, Message{})
	if rows < 4 {
		t.Errorf("Soft wrap rows=%d, want ≥ 4 (verify rowsWithMarker invokes wrap)", rows)
	}
}

func TestMeasureOnlyBuf_ZeroRow(t *testing.T) {
	buf := measureOnlyBuf(20, 0)
	_, rows := buf.Size()
	if rows != 0 {
		t.Errorf("rows=%d, want 0 (spec-1.7 R2 D6 HIGH-E 0-row contract)", rows)
	}
}
