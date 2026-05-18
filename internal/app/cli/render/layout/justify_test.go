// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package layout

import "testing"

func TestApplyJustify_Start(t *testing.T) {
	t.Parallel()
	lead, gap := applyJustify(JustifyStart, 80, 30, 3)
	if lead != 0 || gap != 0 {
		t.Errorf("Start: lead=%d gap=%d, want 0/0", lead, gap)
	}
}

func TestApplyJustify_Center(t *testing.T) {
	t.Parallel()
	lead, gap := applyJustify(JustifyCenter, 80, 30, 3)
	if lead != 25 || gap != 0 {
		t.Errorf("Center: lead=%d gap=%d, want 25/0", lead, gap)
	}
}

func TestApplyJustify_End(t *testing.T) {
	t.Parallel()
	lead, gap := applyJustify(JustifyEnd, 80, 30, 3)
	if lead != 50 || gap != 0 {
		t.Errorf("End: lead=%d gap=%d, want 50/0", lead, gap)
	}
}

func TestApplyJustify_SpaceBetween(t *testing.T) {
	t.Parallel()
	// free = 80 - 30 = 50; gap = 50 / (3 - 1) = 25
	lead, gap := applyJustify(JustifySpaceBetween, 80, 30, 3)
	if lead != 0 || gap != 25 {
		t.Errorf("SpaceBetween: lead=%d gap=%d, want 0/25", lead, gap)
	}
}

func TestApplyJustify_SpaceBetween_SingleChild(t *testing.T) {
	t.Parallel()
	lead, gap := applyJustify(JustifySpaceBetween, 80, 10, 1)
	if lead != 0 || gap != 0 {
		t.Errorf("SpaceBetween n=1: lead=%d gap=%d, want 0/0 (fallback)", lead, gap)
	}
}

func TestApplyJustify_SpaceAround(t *testing.T) {
	t.Parallel()
	// free = 80 - 30 = 50; n = 2 → unit = 50 / 4 = 12; gap = 24
	lead, gap := applyJustify(JustifySpaceAround, 80, 30, 2)
	if lead != 12 || gap != 24 {
		t.Errorf("SpaceAround: lead=%d gap=%d, want 12/24", lead, gap)
	}
}

func TestApplyJustify_SpaceAround_Zero(t *testing.T) {
	t.Parallel()
	lead, gap := applyJustify(JustifySpaceAround, 80, 0, 0)
	if lead != 0 || gap != 0 {
		t.Errorf("SpaceAround n=0: lead=%d gap=%d, want 0/0", lead, gap)
	}
}

func TestApplyJustify_Overflow(t *testing.T) {
	t.Parallel()
	// Children sum exceeds parent → free clamps to 0.
	lead, gap := applyJustify(JustifyCenter, 50, 80, 2)
	if lead != 0 || gap != 0 {
		t.Errorf("overflow: lead=%d gap=%d, want 0/0", lead, gap)
	}
}

func TestApplyJustify_UnknownFallsBackToStart(t *testing.T) {
	t.Parallel()
	lead, gap := applyJustify(Justify(255), 80, 30, 3)
	if lead != 0 || gap != 0 {
		t.Errorf("unknown justify fallback: lead=%d gap=%d, want 0/0", lead, gap)
	}
}

// TestLayout_JustifyCenter verifies center justify wires through Layout
// with a Row of two fixed-basis children in a wide parent.
func TestLayout_JustifyCenter(t *testing.T) {
	t.Parallel()
	a := &FlexNode{BasisMode: BasisFixed, Basis: 10}
	b := &FlexNode{BasisMode: BasisFixed, Basis: 10}
	root := &FlexNode{Direction: Row, Justify: JustifyCenter, Children: []*FlexNode{a, b}}
	got, err := NewFlexLayouter().Layout(root, Box{Width: 80, Height: 1})
	if err != nil {
		t.Fatalf("Layout err = %v", err)
	}
	// free = 60; lead = 30; a at x=30, b at x=40.
	if got[a].X != 30 {
		t.Errorf("a.X = %d, want 30 (center leading)", got[a].X)
	}
	if got[b].X != 40 {
		t.Errorf("b.X = %d, want 40", got[b].X)
	}
}

func TestLayout_JustifyEnd(t *testing.T) {
	t.Parallel()
	a := &FlexNode{BasisMode: BasisFixed, Basis: 5}
	root := &FlexNode{Direction: Row, Justify: JustifyEnd, Children: []*FlexNode{a}}
	got, err := NewFlexLayouter().Layout(root, Box{Width: 80, Height: 1})
	if err != nil {
		t.Fatalf("Layout err = %v", err)
	}
	if got[a].X != 75 {
		t.Errorf("a.X = %d, want 75 (end-justified)", got[a].X)
	}
}

func TestLayout_JustifySpaceBetween(t *testing.T) {
	t.Parallel()
	a := &FlexNode{BasisMode: BasisFixed, Basis: 10}
	b := &FlexNode{BasisMode: BasisFixed, Basis: 10}
	c := &FlexNode{BasisMode: BasisFixed, Basis: 10}
	root := &FlexNode{Direction: Row, Justify: JustifySpaceBetween, Children: []*FlexNode{a, b, c}}
	got, err := NewFlexLayouter().Layout(root, Box{Width: 80, Height: 1})
	if err != nil {
		t.Fatalf("Layout err = %v", err)
	}
	// free = 50; gap = 25; a@0, b@10+25=35, c@45+25=70
	if got[a].X != 0 || got[b].X != 35 || got[c].X != 70 {
		t.Errorf("space-between: a@%d b@%d c@%d, want 0/35/70",
			got[a].X, got[b].X, got[c].X)
	}
}

func TestLayout_JustifySpaceAround_Column(t *testing.T) {
	t.Parallel()
	a := &FlexNode{BasisMode: BasisFixed, Basis: 2}
	b := &FlexNode{BasisMode: BasisFixed, Basis: 2}
	root := &FlexNode{Direction: Column, Justify: JustifySpaceAround, Children: []*FlexNode{a, b}}
	got, err := NewFlexLayouter().Layout(root, Box{Width: 10, Height: 20})
	if err != nil {
		t.Fatalf("Layout err = %v", err)
	}
	// free = 16; unit = 16/4 = 4; gap = 8.
	// a.Y = 4; b.Y = 4 + 2 + 8 = 14.
	if got[a].Y != 4 || got[b].Y != 14 {
		t.Errorf("space-around column: a.Y=%d b.Y=%d, want 4/14", got[a].Y, got[b].Y)
	}
}
