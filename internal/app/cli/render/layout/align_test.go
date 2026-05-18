// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package layout

import "testing"

func TestApplyAlign_Stretch(t *testing.T) {
	t.Parallel()
	size, off := applyAlign(AlignStretch, 10, 3)
	if size != 10 || off != 0 {
		t.Errorf("Stretch: size=%d off=%d, want 10/0", size, off)
	}
}

func TestApplyAlign_Start(t *testing.T) {
	t.Parallel()
	size, off := applyAlign(AlignStart, 10, 3)
	if size != 3 || off != 0 {
		t.Errorf("Start: size=%d off=%d, want 3/0", size, off)
	}
}

func TestApplyAlign_Center(t *testing.T) {
	t.Parallel()
	size, off := applyAlign(AlignCenter, 10, 4)
	if size != 4 || off != 3 {
		t.Errorf("Center: size=%d off=%d, want 4/3", size, off)
	}
}

func TestApplyAlign_End(t *testing.T) {
	t.Parallel()
	size, off := applyAlign(AlignEnd, 10, 3)
	if size != 3 || off != 7 {
		t.Errorf("End: size=%d off=%d, want 3/7", size, off)
	}
}

func TestApplyAlign_ZeroIntrinsic(t *testing.T) {
	t.Parallel()
	// childIntrinsicCross == 0 → fall back to parentCross so leaves
	// without a Measure callback still appear visible.
	size, off := applyAlign(AlignStart, 10, 0)
	if size != 10 || off != 0 {
		t.Errorf("zero intrinsic: size=%d off=%d, want 10/0", size, off)
	}
}

func TestApplyAlign_OversizeIntrinsic(t *testing.T) {
	t.Parallel()
	// childIntrinsicCross > parentCross → clip to parentCross.
	size, off := applyAlign(AlignEnd, 10, 20)
	if size != 10 || off != 0 {
		t.Errorf("oversize: size=%d off=%d, want 10/0", size, off)
	}
}

func TestApplyAlign_UnknownFallsBackToStretch(t *testing.T) {
	t.Parallel()
	size, off := applyAlign(Align(255), 10, 3)
	if size != 10 || off != 0 {
		t.Errorf("unknown align fallback: size=%d off=%d, want 10/0", size, off)
	}
}

// Integration: align wires through Layout. Row direction, Align=Center.
func TestLayout_AlignCenter_Row(t *testing.T) {
	t.Parallel()
	a := &FlexNode{Measure: func() (int, int) { return 5, 2 }}
	root := &FlexNode{
		Direction: Row,
		Align:     AlignCenter,
		Children:  []*FlexNode{a},
	}
	got, err := NewFlexLayouter().Layout(root, Box{Width: 80, Height: 10})
	if err != nil {
		t.Fatalf("Layout err = %v", err)
	}
	// Cross axis = height = 10; child intrinsic h = 2; offset = (10-2)/2 = 4.
	if got[a].Height != 2 {
		t.Errorf("AlignCenter height = %d, want 2", got[a].Height)
	}
	if got[a].Y != 4 {
		t.Errorf("AlignCenter Y = %d, want 4", got[a].Y)
	}
}

func TestLayout_AlignEnd_Column(t *testing.T) {
	t.Parallel()
	a := &FlexNode{Measure: func() (int, int) { return 3, 5 }}
	root := &FlexNode{
		Direction: Column,
		Align:     AlignEnd,
		Children:  []*FlexNode{a},
	}
	got, err := NewFlexLayouter().Layout(root, Box{Width: 10, Height: 20})
	if err != nil {
		t.Fatalf("Layout err = %v", err)
	}
	// Cross axis = width = 10; child intrinsic w = 3; X offset = 10-3 = 7.
	if got[a].Width != 3 {
		t.Errorf("AlignEnd width = %d, want 3", got[a].Width)
	}
	if got[a].X != 7 {
		t.Errorf("AlignEnd X = %d, want 7", got[a].X)
	}
}

func TestLayout_AlignStretch_Default(t *testing.T) {
	t.Parallel()
	// Default (zero-value Align == AlignStretch) — leaf fills cross axis.
	a := &FlexNode{Measure: func() (int, int) { return 5, 2 }}
	root := &FlexNode{Direction: Row, Children: []*FlexNode{a}}
	got, err := NewFlexLayouter().Layout(root, Box{Width: 80, Height: 10})
	if err != nil {
		t.Fatalf("Layout err = %v", err)
	}
	if got[a].Height != 10 {
		t.Errorf("AlignStretch height = %d, want 10 (full cross)", got[a].Height)
	}
}
