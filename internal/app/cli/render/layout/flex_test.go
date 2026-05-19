// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package layout

import "testing"

func TestFlexNodeIntrinsic(t *testing.T) {
	t.Parallel()
	var nilNode *FlexNode
	if w, h := nilNode.Intrinsic(); w != 0 || h != 0 {
		t.Fatalf("nil FlexNode Intrinsic = %d,%d, want 0,0", w, h)
	}
	empty := NewFlexNode()
	if w, h := empty.Intrinsic(); w != 0 || h != 0 {
		t.Fatalf("empty FlexNode Intrinsic = %d,%d, want 0,0", w, h)
	}
	leaf := &FlexNode{Measure: func() (int, int) { return 7, 3 }}
	if w, h := leaf.Intrinsic(); w != 7 || h != 3 {
		t.Fatalf("measured FlexNode Intrinsic = %d,%d, want 7,3", w, h)
	}
}

func TestLayout_Nil(t *testing.T) {
	t.Parallel()
	l := NewFlexLayouter()
	got, err := l.Layout(nil, Box{Width: 80, Height: 24})
	if err != nil {
		t.Fatalf("Layout(nil) err = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Errorf("Layout(nil) = %v, want empty map", got)
	}
}

func TestLayout_SingleLeaf(t *testing.T) {
	t.Parallel()
	leaf := &FlexNode{}
	l := NewFlexLayouter()
	got, err := l.Layout(leaf, Box{Width: 80, Height: 24})
	if err != nil {
		t.Fatalf("Layout err = %v", err)
	}
	want := Box{X: 0, Y: 0, Width: 80, Height: 24}
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
	got, err := NewFlexLayouter().Layout(root, Box{Width: 80, Height: 1})
	if err != nil {
		t.Fatalf("Layout err = %v", err)
	}
	assertBox(t, "a", got[a], Box{X: 0, Y: 0, Width: 20, Height: 1})
	assertBox(t, "b", got[b], Box{X: 20, Y: 0, Width: 30, Height: 1})
	assertBox(t, "c", got[c], Box{X: 50, Y: 0, Width: 30, Height: 1})
}

func TestLayout_RowGrowSpacer(t *testing.T) {
	t.Parallel()
	// 5 + grow-spacer + 5 in 80 cols → spacer takes 70
	left := &FlexNode{BasisMode: BasisFixed, Basis: 5}
	spacer := &FlexNode{BasisMode: BasisFixed, Basis: 0, Grow: 1}
	right := &FlexNode{BasisMode: BasisFixed, Basis: 5}
	root := &FlexNode{Direction: Row, Children: []*FlexNode{left, spacer, right}}
	got, err := NewFlexLayouter().Layout(root, Box{Width: 80, Height: 1})
	if err != nil {
		t.Fatalf("Layout err = %v", err)
	}
	assertBox(t, "left", got[left], Box{X: 0, Y: 0, Width: 5, Height: 1})
	assertBox(t, "spacer", got[spacer], Box{X: 5, Y: 0, Width: 70, Height: 1})
	assertBox(t, "right", got[right], Box{X: 75, Y: 0, Width: 5, Height: 1})
}

func TestLayout_GrowLeftoverGoesToLargestGrow(t *testing.T) {
	t.Parallel()
	a := &FlexNode{BasisMode: BasisFixed, Basis: 0, Grow: 2}
	b := &FlexNode{BasisMode: BasisFixed, Basis: 0, Grow: 1}
	root := &FlexNode{Direction: Row, Children: []*FlexNode{a, b}}
	got, err := NewFlexLayouter().Layout(root, Box{Width: 5, Height: 1})
	if err != nil {
		t.Fatalf("Layout err = %v", err)
	}
	assertBox(t, "a", got[a], Box{X: 0, Y: 0, Width: 4, Height: 1})
	assertBox(t, "b", got[b], Box{X: 4, Y: 0, Width: 1, Height: 1})
}

func TestLayout_RowShrinkOverflow(t *testing.T) {
	t.Parallel()
	// 50 + 50 in 80 cols (overflow 20), shrink ratio 1:1 → each loses 10
	a := &FlexNode{BasisMode: BasisFixed, Basis: 50, Shrink: 1}
	b := &FlexNode{BasisMode: BasisFixed, Basis: 50, Shrink: 1}
	root := &FlexNode{Direction: Row, Children: []*FlexNode{a, b}}
	got, err := NewFlexLayouter().Layout(root, Box{Width: 80, Height: 1})
	if err != nil {
		t.Fatalf("Layout err = %v", err)
	}
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
	got, err := NewFlexLayouter().Layout(root, Box{Width: 50, Height: 1})
	if err != nil {
		t.Fatalf("Layout err = %v", err)
	}
	if got[a].Width != 100 {
		t.Errorf("no-shrink: a.W=%d, want 100 (parent overflows)", got[a].Width)
	}
}

func TestLayout_GrowSumZeroNoDivByZero(t *testing.T) {
	t.Parallel()
	a := &FlexNode{BasisMode: BasisFixed, Basis: 10}
	b := &FlexNode{BasisMode: BasisFixed, Basis: 20}
	root := &FlexNode{Direction: Row, Children: []*FlexNode{a, b}}
	got, err := NewFlexLayouter().Layout(root, Box{Width: 80, Height: 1})
	if err != nil {
		t.Fatalf("Layout err = %v", err)
	}
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
	got, err := NewFlexLayouter().Layout(root, Box{Width: 40, Height: 24})
	if err != nil {
		t.Fatalf("Layout err = %v", err)
	}
	assertBox(t, "a", got[a], Box{X: 0, Y: 0, Width: 40, Height: 1})
	assertBox(t, "b", got[b], Box{X: 0, Y: 1, Width: 40, Height: 5})
	assertBox(t, "c", got[c], Box{X: 0, Y: 6, Width: 40, Height: 2})
}

func TestLayout_AutoBasisFromIntrinsic(t *testing.T) {
	t.Parallel()
	a := &FlexNode{Measure: func() (int, int) { return 7, 1 }}
	b := &FlexNode{Measure: func() (int, int) { return 13, 1 }}
	root := &FlexNode{Direction: Row, Children: []*FlexNode{a, b}}
	got, err := NewFlexLayouter().Layout(root, Box{Width: 80, Height: 1})
	if err != nil {
		t.Fatalf("Layout err = %v", err)
	}
	if got[a].Width != 7 || got[b].Width != 13 {
		t.Errorf("auto basis: a.W=%d b.W=%d, want 7/13", got[a].Width, got[b].Width)
	}
}

func TestLayout_NestedColumnRow(t *testing.T) {
	t.Parallel()
	headerL := &FlexNode{BasisMode: BasisFixed, Basis: 10}
	headerR := &FlexNode{BasisMode: BasisFixed, Basis: 30}
	header := &FlexNode{Direction: Row, BasisMode: BasisFixed, Basis: 1, Children: []*FlexNode{headerL, headerR}}
	bodyL := &FlexNode{BasisMode: BasisFixed, Basis: 5}
	bodyR := &FlexNode{BasisMode: BasisFixed, Basis: 35}
	body := &FlexNode{Direction: Row, Grow: 1, BasisMode: BasisFixed, Basis: 0, Children: []*FlexNode{bodyL, bodyR}}
	root := &FlexNode{Direction: Column, Children: []*FlexNode{header, body}}
	got, err := NewFlexLayouter().Layout(root, Box{Width: 40, Height: 24})
	if err != nil {
		t.Fatalf("Layout err = %v", err)
	}
	assertBox(t, "header", got[header], Box{X: 0, Y: 0, Width: 40, Height: 1})
	assertBox(t, "body", got[body], Box{X: 0, Y: 1, Width: 40, Height: 23})
	assertBox(t, "headerL", got[headerL], Box{X: 0, Y: 0, Width: 10, Height: 1})
	assertBox(t, "headerR", got[headerR], Box{X: 10, Y: 0, Width: 30, Height: 1})
	assertBox(t, "bodyL", got[bodyL], Box{X: 0, Y: 1, Width: 5, Height: 23})
	assertBox(t, "bodyR", got[bodyR], Box{X: 5, Y: 1, Width: 35, Height: 23})
}

func TestLayout_EmptyChildren(t *testing.T) {
	t.Parallel()
	root := &FlexNode{Direction: Row, Children: []*FlexNode{}}
	got, err := NewFlexLayouter().Layout(root, Box{Width: 40, Height: 10})
	if err != nil {
		t.Fatalf("Layout err = %v", err)
	}
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

// TestLayout_FloatRemainderTieBreak documents the deterministic
// tie-break: with 3 equal-grow children sharing 10 cells, integer
// truncation gives 3+3+3=9; the leftover 1 cell goes to the first
// child with the largest Grow (here child a, ties broken by order).
func TestLayout_FloatRemainderTieBreak(t *testing.T) {
	t.Parallel()
	a := &FlexNode{BasisMode: BasisFixed, Basis: 0, Grow: 1}
	b := &FlexNode{BasisMode: BasisFixed, Basis: 0, Grow: 1}
	c := &FlexNode{BasisMode: BasisFixed, Basis: 0, Grow: 1}
	root := &FlexNode{Direction: Row, Children: []*FlexNode{a, b, c}}
	got, err := NewFlexLayouter().Layout(root, Box{Width: 10, Height: 1})
	if err != nil {
		t.Fatalf("Layout err = %v", err)
	}
	if got[a].Width+got[b].Width+got[c].Width != 10 {
		t.Errorf("widths must sum to viewport: %d+%d+%d != 10",
			got[a].Width, got[b].Width, got[c].Width)
	}
	if got[a].Width != 4 {
		t.Errorf("leftover should go to a: a.W=%d, want 4", got[a].Width)
	}
}

func TestLayout_AutoBasisUnderTooSmallViewport(t *testing.T) {
	t.Parallel()
	// Sum of intrinsic = 20 but viewport.Width = 10 and Shrink is
	// explicitly disabled. Per Yoga standard, allow overflow. This test
	// documents no-shrink behavior, not CC Ink defaults; use NewFlexNode()
	// for CC-aligned Shrink=1 defaults.
	a := &FlexNode{Shrink: 0, Measure: func() (int, int) { return 10, 1 }}
	b := &FlexNode{Shrink: 0, Measure: func() (int, int) { return 10, 1 }}
	root := &FlexNode{Direction: Row, Children: []*FlexNode{a, b}}
	got, err := NewFlexLayouter().Layout(root, Box{Width: 10, Height: 1})
	if err != nil {
		t.Fatalf("Layout err = %v", err)
	}
	if got[a].Width != 10 || got[b].Width != 10 {
		t.Errorf("overflow allowed: a.W=%d b.W=%d, want 10/10", got[a].Width, got[b].Width)
	}
}

func TestLayout_NewFlexNodeShrinkDefaultAppliesToLayout(t *testing.T) {
	t.Parallel()
	a := NewFlexNode()
	a.Measure = func() (int, int) { return 10, 1 }
	b := NewFlexNode()
	b.Measure = func() (int, int) { return 10, 1 }
	root := NewFlexNode()
	root.Children = []*FlexNode{a, b}
	got, err := NewFlexLayouter().Layout(root, Box{Width: 10, Height: 1})
	if err != nil {
		t.Fatalf("Layout err = %v", err)
	}
	if got[a].Width != 5 || got[b].Width != 5 {
		t.Errorf("NewFlexNode shrink defaults: a.W=%d b.W=%d, want 5/5", got[a].Width, got[b].Width)
	}
}

func TestLayout_LeafIntrinsicReturnsContainerBoxes(t *testing.T) {
	t.Parallel()
	// All-node return: the map must contain root + every child.
	a := &FlexNode{BasisMode: BasisFixed, Basis: 10}
	b := &FlexNode{BasisMode: BasisFixed, Basis: 10}
	root := &FlexNode{Direction: Row, Children: []*FlexNode{a, b}}
	got, err := NewFlexLayouter().Layout(root, Box{Width: 80, Height: 1})
	if err != nil {
		t.Fatalf("Layout err = %v", err)
	}
	if len(got) != 3 {
		t.Errorf("expected 3 entries (root + 2 children), got %d", len(got))
	}
	if _, ok := got[root]; !ok {
		t.Errorf("root entry missing from result map")
	}
}

func TestLayout_NewFlexNodeDefaults(t *testing.T) {
	t.Parallel()
	n := NewFlexNode()
	if n.Direction != Row {
		t.Errorf("Direction default = %v, want Row (CC Ink Box.tsx L109)", n.Direction)
	}
	if n.Grow != 0 {
		t.Errorf("Grow default = %v, want 0 (CC Ink Box.tsx L110)", n.Grow)
	}
	if n.Shrink != 1 {
		t.Errorf("Shrink default = %v, want 1 (CC Ink Box.tsx L111)", n.Shrink)
	}
	if n.Justify != JustifyStart {
		t.Errorf("Justify default = %v, want Start", n.Justify)
	}
	if n.Align != AlignStretch {
		t.Errorf("Align default = %v, want Stretch", n.Align)
	}
	if n.BasisMode != BasisAuto {
		t.Errorf("BasisMode default = %v, want Auto", n.BasisMode)
	}
}

func TestLayout_NonFlexNodeFallback(t *testing.T) {
	t.Parallel()
	// Non-FlexNode Node implementations are treated as a single leaf
	// using its intrinsic size clamped to the viewport. This keeps the
	// Layouter interface usable for caller-defined Node types.
	root := &fakeNode{w: 5, h: 2}
	got, err := NewFlexLayouter().Layout(root, Box{Width: 80, Height: 24})
	if err != nil {
		t.Fatalf("Layout err = %v", err)
	}
	if got[root].Width != 5 || got[root].Height != 2 {
		t.Errorf("non-flex leaf box = %+v, want 5x2", got[root])
	}
}

// --- helpers ---------------------------------------------------------

func assertBox(t *testing.T, name string, got, want Box) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %+v, want %+v", name, got, want)
	}
}
