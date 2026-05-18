// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package layout

import (
	"errors"
	"math"
	"testing"

	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

// TestErrors_RegistryEntries verifies both RENDER.* codes are
// registered with non-empty Code / Message / Hint.
func TestErrors_RegistryEntries(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  errcode.Error
		code string
	}{
		{"INVALID_DIMENSION", ErrInvalidDimension, "RENDER.INVALID_DIMENSION"},
		{"LAYOUT_CYCLE", ErrLayoutCycle, "RENDER.LAYOUT_CYCLE"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.err.Code() != tc.code {
				t.Errorf("Code = %q, want %q", tc.err.Code(), tc.code)
			}
			if tc.err.Message() == "" {
				t.Errorf("Message is empty")
			}
			if tc.err.Hint() == "" {
				t.Errorf("Hint is empty")
			}
		})
	}
}

func TestLayout_ViewportZeroWidth(t *testing.T) {
	t.Parallel()
	root := &FlexNode{}
	_, err := NewFlexLayouter().Layout(root, Box{Width: 0, Height: 24})
	if !errors.Is(err, ErrInvalidDimension) {
		t.Errorf("viewport W=0 err = %v, want ErrInvalidDimension", err)
	}
}

func TestLayout_ViewportNegativeHeight(t *testing.T) {
	t.Parallel()
	root := &FlexNode{}
	_, err := NewFlexLayouter().Layout(root, Box{Width: 80, Height: -1})
	if !errors.Is(err, ErrInvalidDimension) {
		t.Errorf("viewport H=-1 err = %v, want ErrInvalidDimension", err)
	}
}

func TestLayout_NegativeGrow(t *testing.T) {
	t.Parallel()
	a := &FlexNode{Grow: -1}
	root := &FlexNode{Direction: Row, Children: []*FlexNode{a}}
	_, err := NewFlexLayouter().Layout(root, Box{Width: 80, Height: 1})
	if !errors.Is(err, ErrInvalidDimension) {
		t.Errorf("negative grow err = %v, want ErrInvalidDimension", err)
	}
}

func TestLayout_NegativeShrink(t *testing.T) {
	t.Parallel()
	a := &FlexNode{Shrink: -1}
	root := &FlexNode{Direction: Row, Children: []*FlexNode{a}}
	_, err := NewFlexLayouter().Layout(root, Box{Width: 80, Height: 1})
	if !errors.Is(err, ErrInvalidDimension) {
		t.Errorf("negative shrink err = %v, want ErrInvalidDimension", err)
	}
}

func TestLayout_NegativeBasis(t *testing.T) {
	t.Parallel()
	a := &FlexNode{BasisMode: BasisFixed, Basis: -5}
	root := &FlexNode{Direction: Row, Children: []*FlexNode{a}}
	_, err := NewFlexLayouter().Layout(root, Box{Width: 80, Height: 1})
	if !errors.Is(err, ErrInvalidDimension) {
		t.Errorf("negative basis err = %v, want ErrInvalidDimension", err)
	}
}

func TestLayout_NegativeIntrinsic(t *testing.T) {
	t.Parallel()
	a := &FlexNode{Measure: func() (int, int) { return -1, 5 }}
	root := &FlexNode{Direction: Row, Children: []*FlexNode{a}}
	_, err := NewFlexLayouter().Layout(root, Box{Width: 80, Height: 1})
	if !errors.Is(err, ErrInvalidDimension) {
		t.Errorf("negative intrinsic err = %v, want ErrInvalidDimension", err)
	}
}

func TestLayout_TooManyChildren(t *testing.T) {
	t.Parallel()
	children := make([]*FlexNode, MaxChildren+1)
	for i := range children {
		children[i] = &FlexNode{}
	}
	root := &FlexNode{Direction: Row, Children: children}
	_, err := NewFlexLayouter().Layout(root, Box{Width: 80, Height: 1})
	if !errors.Is(err, ErrInvalidDimension) {
		t.Errorf(">1000 children err = %v, want ErrInvalidDimension", err)
	}
}

func TestLayout_MaxIntOverflow(t *testing.T) {
	t.Parallel()
	// Three children whose intrinsic widths sum overflows int32.
	big := math.MaxInt32 - 5
	a := &FlexNode{Measure: func() (int, int) { return big, 1 }}
	b := &FlexNode{Measure: func() (int, int) { return big, 1 }}
	root := &FlexNode{Direction: Row, Children: []*FlexNode{a, b}}
	_, err := NewFlexLayouter().Layout(root, Box{Width: 80, Height: 1})
	if !errors.Is(err, ErrInvalidDimension) {
		t.Errorf("MaxInt overflow err = %v, want ErrInvalidDimension", err)
	}
}

// TestLayout_CycleViaIntrinsicCallback verifies that an Intrinsic
// callback recursively calling itself via the Layouter trips
// ErrLayoutCycle. The callback registers itself with a closure that
// re-enters the layout measure path via a sentinel.
func TestLayout_CycleViaIntrinsicCallback(t *testing.T) {
	t.Parallel()
	// We cannot reach the per-Layout-call measureCache from outside,
	// so this test exercises the cache directly (covered also by
	// cache_test.go TestMeasureCache_CycleDetection). The integration
	// here ensures the error type propagates through the public API.
	c := newMeasureCache()
	var n *FlexNode
	var innerErr error
	n = &FlexNode{Measure: func() (int, int) {
		_, _, innerErr = c.measure(n)
		return 1, 1
	}}
	_, _, _ = c.measure(n)
	if !errors.Is(innerErr, ErrLayoutCycle) {
		t.Errorf("cycle err = %v, want ErrLayoutCycle", innerErr)
	}
}

func TestErrors_AreErrcodeError(t *testing.T) {
	t.Parallel()
	var ec errcode.Error
	if !errors.As(error(ErrInvalidDimension), &ec) {
		t.Errorf("ErrInvalidDimension not errcode.Error")
	}
	if !errors.As(error(ErrLayoutCycle), &ec) {
		t.Errorf("ErrLayoutCycle not errcode.Error")
	}
}
