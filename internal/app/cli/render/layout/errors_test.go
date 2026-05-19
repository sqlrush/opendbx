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

func TestLayout_NilChild(t *testing.T) {
	t.Parallel()
	root := NewFlexNode()
	root.Children = []*FlexNode{nil}
	_, err := NewFlexLayouter().Layout(root, Box{Width: 80, Height: 1})
	if !errors.Is(err, ErrInvalidDimension) {
		t.Errorf("nil child err = %v, want ErrInvalidDimension", err)
	}
}

func TestLayout_ViewportExceedsInt32(t *testing.T) {
	t.Parallel()
	root := NewFlexNode()
	_, err := NewFlexLayouter().Layout(root, Box{Width: math.MaxInt32 + 1, Height: 1})
	if !errors.Is(err, ErrInvalidDimension) {
		t.Errorf("oversized viewport err = %v, want ErrInvalidDimension", err)
	}
}

func TestLayout_DuplicateChildReference(t *testing.T) {
	t.Parallel()
	child := NewFlexNode()
	root := NewFlexNode()
	root.Children = []*FlexNode{child, child}
	_, err := NewFlexLayouter().Layout(root, Box{Width: 80, Height: 1})
	if !errors.Is(err, ErrInvalidDimension) {
		t.Errorf("duplicate child err = %v, want ErrInvalidDimension", err)
	}
}

func TestLayout_SelfReferentialChild(t *testing.T) {
	t.Parallel()
	root := NewFlexNode()
	root.Children = []*FlexNode{root}
	_, err := NewFlexLayouter().Layout(root, Box{Width: 80, Height: 1})
	if !errors.Is(err, ErrInvalidDimension) {
		t.Errorf("self-referential child err = %v, want ErrInvalidDimension", err)
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
// callback recursively calling Layout on the same node through the same
// Layouter instance trips ErrLayoutCycle at the public API boundary.
func TestLayout_CycleViaIntrinsicCallback(t *testing.T) {
	t.Parallel()
	l := NewFlexLayouter()
	var n *FlexNode
	var innerErr error
	n = NewFlexNode()
	n.Measure = func() (int, int) {
		_, innerErr = l.Layout(n, Box{Width: 1, Height: 1})
		return 1, 1
	}

	_, err := l.Layout(n, Box{Width: 1, Height: 1})
	if err != nil {
		t.Fatalf("outer Layout err = %v, want nil; inner re-entry should fail", err)
	}
	if !errors.Is(innerErr, ErrLayoutCycle) {
		t.Errorf("cycle err = %v, want ErrLayoutCycle", innerErr)
	}
}

func TestLayout_ChildMeasureReentryViaPublicLayout(t *testing.T) {
	t.Parallel()
	l := NewFlexLayouter()
	child := NewFlexNode()
	root := NewFlexNode()
	root.Children = []*FlexNode{child}

	calls := 0
	var innerErr error
	child.Measure = func() (int, int) {
		calls++
		if calls == 1 {
			_, innerErr = l.Layout(child, Box{Width: 1, Height: 1})
		}
		return 1, 1
	}

	_, err := l.Layout(root, Box{Width: 1, Height: 1})
	if err != nil {
		t.Fatalf("outer Layout err = %v, want nil; child re-entry should fail", err)
	}
	if !errors.Is(innerErr, ErrLayoutCycle) {
		t.Errorf("child re-entry err = %v, want ErrLayoutCycle", innerErr)
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
