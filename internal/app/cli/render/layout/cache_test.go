// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package layout

import (
	"errors"
	"testing"

	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

// TestMeasureCache_SingleCallPerLeaf verifies the spec-1.1 R2-7
// call-count contract: each leaf's Measure callback is invoked at
// most once per Layout() call, even if intrinsicPass or distributePass
// would otherwise re-read it (instrumented counter test).
func TestMeasureCache_SingleCallPerLeaf(t *testing.T) {
	t.Parallel()
	var callsA, callsB int
	a := &FlexNode{Measure: func() (int, int) { callsA++; return 5, 1 }}
	b := &FlexNode{Measure: func() (int, int) { callsB++; return 7, 1 }}
	root := &FlexNode{Direction: Row, Children: []*FlexNode{a, b}}
	if _, err := NewFlexLayouter().Layout(root, Box{Width: 80, Height: 1}); err != nil {
		t.Fatalf("Layout err = %v", err)
	}
	if callsA != 1 {
		t.Errorf("leaf a Measure call-count = %d, want 1 (single-call-per-leaf contract)", callsA)
	}
	if callsB != 1 {
		t.Errorf("leaf b Measure call-count = %d, want 1", callsB)
	}
}

// TestMeasureCache_HitOnSecondMeasure verifies the in-memory cache
// returns the stored result without re-invoking Measure when the same
// node is requested twice through the same cache.
func TestMeasureCache_HitOnSecondMeasure(t *testing.T) {
	t.Parallel()
	var calls int
	n := &FlexNode{Measure: func() (int, int) { calls++; return 3, 4 }}
	c := newMeasureCache()
	w, h, err := c.measure(n)
	if err != nil {
		t.Fatalf("measure err = %v", err)
	}
	if w != 3 || h != 4 {
		t.Errorf("first measure = %d,%d, want 3,4", w, h)
	}
	w, h, err = c.measure(n)
	if err != nil {
		t.Fatalf("second measure err = %v", err)
	}
	if w != 3 || h != 4 {
		t.Errorf("second measure = %d,%d, want 3,4", w, h)
	}
	if calls != 1 {
		t.Errorf("Measure call count = %d, want 1 (cache hit)", calls)
	}
}

// TestMeasureCache_CycleDetection verifies that a Measure callback
// re-entering measurement of the same Node returns ErrLayoutCycle.
func TestMeasureCache_CycleDetection(t *testing.T) {
	t.Parallel()
	c := newMeasureCache()
	var n *FlexNode
	var innerErr error
	n = &FlexNode{Measure: func() (int, int) {
		// Re-enter measurement of self via the same cache.
		_, _, innerErr = c.measure(n)
		return 1, 1
	}}
	_, _, err := c.measure(n)
	if err != nil {
		t.Fatalf("outer measure err = %v (should succeed; cycle is in inner call)", err)
	}
	if !errors.Is(innerErr, ErrLayoutCycle) {
		t.Errorf("inner re-entry should return ErrLayoutCycle, got %v", innerErr)
	}
	var ec errcode.Error
	if !errors.As(innerErr, &ec) {
		t.Fatalf("inner err not errcode.Error: %v", innerErr)
	}
	if ec.Code() != "RENDER.LAYOUT_CYCLE" {
		t.Errorf("err Code = %q, want RENDER.LAYOUT_CYCLE", ec.Code())
	}
}
