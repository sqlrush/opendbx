// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

//go:build spike
// +build spike

package spike

import (
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/layout"
)

func TestLayout_Nil(t *testing.T) {
	t.Parallel()
	got := Layout(nil, layout.Box{Width: 80, Height: 24})
	if len(got) != 0 {
		t.Errorf("Layout(nil) = %v, want empty map", got)
	}
}

func TestLayout_SingleLeaf(t *testing.T) {
	t.Parallel()
	leaf := &FlexNode{}
	got := Layout(leaf, layout.Box{Width: 80, Height: 24})
	want := layout.Box{X: 0, Y: 0, Width: 80, Height: 24}
	if got[leaf] != want {
		t.Errorf("single leaf box = %+v, want %+v", got[leaf], want)
	}
}

func TestLayout_RowFixedBasis(t *testing.T) {
	t.Parallel()
	a := &FlexNode{BasisMode: BasisFixed, Basis: 20}
	b := &FlexNode{BasisMode: BasisFixed, Basis: 30}
	c := &FlexNode{BasisMode: BasisFixed, Basis: 30}
	root := &FlexNode{Direction: Row, Children: []*FlexNode{a, b, c}}
	got := Layout(root, layout.Box{Width: 80, Height: 1})
	assertBox(t, "a", got[a], layout.Box{X: 0, Y: 0, Width: 20, Height: 1})
	assertBox(t, "b", got[b], layout.Box{X: 20, Y: 0, Width: 30, Height: 1})
	assertBox(t, "c", got[c], layout.Box{X: 50, Y: 0, Width: 30, Height: 1})
}

func TestLayout_RowGrowSpacer(t *testing.T) {
	t.Parallel()
	// 5 + grow-spacer + 5 in 80 cols → spacer takes 70
	left := &FlexNode{BasisMode: BasisFixed, Basis: 5}
	spacer := &FlexNode{BasisMode: BasisFixed, Basis: 0, Grow: 1}
	right := &FlexNode{BasisMode: BasisFixed, Basis: 5}
	root := &FlexNode{Direction: Row, Children: []*FlexNode{left, spacer, right}}
	got := Layout(root, layout.Box{Width: 80, Height: 1})
	assertBox(t, "left", got[left], layout.Box{X: 0, Y: 0, Width: 5, Height: 1})
	assertBox(t, "spacer", got[spacer], layout.Box{X: 5, Y: 0, Width: 70, Height: 1})
	assertBox(t, "right", got[right], layout.Box{X: 75, Y: 0, Width: 5, Height: 1})
}

func TestLayout_RowShrinkOverflow(t *testing.T) {
	t.Parallel()
	// 50 + 50 in 80 cols (overflow 20), shrink ratio 1:1 → each loses 10
	a := &FlexNode{BasisMode: BasisFixed, Basis: 50, Shrink: 1}
	b := &FlexNode{BasisMode: BasisFixed, Basis: 50, Shrink: 1}
	root := &FlexNode{Direction: Row, Children: []*FlexNode{a, b}}
	got := Layout(root, layout.Box{Width: 80, Height: 1})
	if got[a].Width != 40 || got[b].Width != 40 {
		t.Errorf("shrink: a.W=%d b.W=%d, want 40/40", got[a].Width, got[b].Width)
	}
	if got[a].X != 0 || got[b].X != 40 {
		t.Errorf("shrink positions: a.X=%d b.X=%d, want 0/40", got[a].X, got[b].X)
	}
}

func TestLayout_ShrinkBasisZeroNoDivByZero(t *testing.T) {
	t.Parallel()
	// shrink=0 means no shrink; child stays at basis even with overflow.
	a := &FlexNode{BasisMode: BasisFixed, Basis: 100}
	root := &FlexNode{Direction: Row, Children: []*FlexNode{a}}
	got := Layout(root, layout.Box{Width: 50, Height: 1})
	if got[a].Width != 100 {
		t.Errorf("no-shrink: a.W=%d, want 100 (parent overflows)", got[a].Width)
	}
}

func TestLayout_GrowSumZeroNoDivByZero(t *testing.T) {
	t.Parallel()
	// grow=0 on all children; remainder unused (no panic, no div by zero).
	a := &FlexNode{BasisMode: BasisFixed, Basis: 10}
	b := &FlexNode{BasisMode: BasisFixed, Basis: 20}
	root := &FlexNode{Direction: Row, Children: []*FlexNode{a, b}}
	got := Layout(root, layout.Box{Width: 80, Height: 1})
	if got[a].Width != 10 || got[b].Width != 20 {
		t.Errorf("no-grow stays at basis: %d/%d, want 10/20", got[a].Width, got[b].Width)
	}
}

func TestLayout_ColumnFixedBasis(t *testing.T) {
	t.Parallel()
	a := &FlexNode{BasisMode: BasisFixed, Basis: 1}
	b := &FlexNode{BasisMode: BasisFixed, Basis: 5}
	c := &FlexNode{BasisMode: BasisFixed, Basis: 2}
	root := &FlexNode{Direction: Column, Children: []*FlexNode{a, b, c}}
	got := Layout(root, layout.Box{Width: 40, Height: 24})
	assertBox(t, "a", got[a], layout.Box{X: 0, Y: 0, Width: 40, Height: 1})
	assertBox(t, "b", got[b], layout.Box{X: 0, Y: 1, Width: 40, Height: 5})
	assertBox(t, "c", got[c], layout.Box{X: 0, Y: 6, Width: 40, Height: 2})
}

func TestLayout_AutoBasisFromIntrinsic(t *testing.T) {
	t.Parallel()
	// Auto basis pulls from Intrinsic().
	a := &FlexNode{Intrinsic: func() (int, int) { return 7, 1 }}
	b := &FlexNode{Intrinsic: func() (int, int) { return 13, 1 }}
	root := &FlexNode{Direction: Row, Children: []*FlexNode{a, b}}
	got := Layout(root, layout.Box{Width: 80, Height: 1})
	if got[a].Width != 7 || got[b].Width != 13 {
		t.Errorf("auto basis: a.W=%d b.W=%d, want 7/13", got[a].Width, got[b].Width)
	}
}

func TestLayout_NestedColumnRow(t *testing.T) {
	t.Parallel()
	// Column root with two Row children (header + body), body grows.
	headerL := &FlexNode{BasisMode: BasisFixed, Basis: 10}
	headerR := &FlexNode{BasisMode: BasisFixed, Basis: 30}
	header := &FlexNode{
		Direction: Row,
		BasisMode: BasisFixed, Basis: 1,
		Children: []*FlexNode{headerL, headerR},
	}
	bodyL := &FlexNode{BasisMode: BasisFixed, Basis: 5}
	bodyR := &FlexNode{BasisMode: BasisFixed, Basis: 35}
	body := &FlexNode{
		Direction: Row,
		Grow:      1,
		BasisMode: BasisFixed, Basis: 0,
		Children: []*FlexNode{bodyL, bodyR},
	}
	root := &FlexNode{Direction: Column, Children: []*FlexNode{header, body}}
	got := Layout(root, layout.Box{Width: 40, Height: 24})
	assertBox(t, "header", got[header], layout.Box{X: 0, Y: 0, Width: 40, Height: 1})
	assertBox(t, "body", got[body], layout.Box{X: 0, Y: 1, Width: 40, Height: 23})
	assertBox(t, "headerL", got[headerL], layout.Box{X: 0, Y: 0, Width: 10, Height: 1})
	assertBox(t, "headerR", got[headerR], layout.Box{X: 10, Y: 0, Width: 30, Height: 1})
	assertBox(t, "bodyL", got[bodyL], layout.Box{X: 0, Y: 1, Width: 5, Height: 23})
	assertBox(t, "bodyR", got[bodyR], layout.Box{X: 5, Y: 1, Width: 35, Height: 23})
}

func TestLayout_LeafIntrinsicCrossUsed(t *testing.T) {
	t.Parallel()
	// Row leaves with Intrinsic returning various heights; container's
	// intrinsic height is max(child heights).
	a := &FlexNode{Intrinsic: func() (int, int) { return 3, 1 }}
	b := &FlexNode{Intrinsic: func() (int, int) { return 3, 2 }}
	c := &FlexNode{Intrinsic: func() (int, int) { return 3, 1 }}
	root := &FlexNode{Direction: Row, Children: []*FlexNode{a, b, c}}
	got := Layout(root, layout.Box{Width: 80, Height: 5})
	// All leaves stretch to parent's cross (5) regardless of intrinsic.
	if got[a].Height != 5 || got[b].Height != 5 || got[c].Height != 5 {
		t.Errorf("cross stretch: heights=%d/%d/%d, want 5/5/5", got[a].Height, got[b].Height, got[c].Height)
	}
}

func TestLayout_EmptyChildren(t *testing.T) {
	t.Parallel()
	root := &FlexNode{Direction: Row, Children: []*FlexNode{}}
	got := Layout(root, layout.Box{Width: 40, Height: 10})
	if got[root].Width != 40 || got[root].Height != 10 {
		t.Errorf("empty container box = %+v", got[root])
	}
}

func TestLayout_DirectionConstantsDistinct(t *testing.T) {
	t.Parallel()
	if Row == Column {
		t.Errorf("Row and Column must be distinct")
	}
	if BasisAuto == BasisFixed {
		t.Errorf("BasisAuto and BasisFixed must be distinct")
	}
}

// --- helpers ---------------------------------------------------------

func assertBox(t *testing.T, name string, got, want layout.Box) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %+v, want %+v", name, got, want)
	}
}
