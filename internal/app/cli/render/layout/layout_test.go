// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package layout

import "testing"

func TestBox_ZeroValue(t *testing.T) {
	t.Parallel()
	b := Box{}
	if b.X != 0 || b.Y != 0 || b.Width != 0 || b.Height != 0 {
		t.Errorf("Box{} not zero: %+v", b)
	}
}

type fakeNode struct{ w, h int }

func (f *fakeNode) Intrinsic() (int, int) { return f.w, f.h }

// fakeLayouter satisfies the spec-1.1 Layouter signature
// `Layout(root Node, viewport Box) (map[Node]Box, error)`. Minimal
// implementation per spec-1.1 R2.1 claude MED-2 (`return map[Node]Box{root:
// Box{...}}, nil`).
type fakeLayouter struct{}

func (fakeLayouter) Layout(root Node, viewport Box) (map[Node]Box, error) {
	w, h := root.Intrinsic()
	return map[Node]Box{root: {X: 0, Y: 0, Width: w, Height: h}}, nil
}

func TestLayouter_InterfaceContract(t *testing.T) {
	t.Parallel()
	var l Layouter = fakeLayouter{}
	root := &fakeNode{w: 20, h: 5}
	boxes, err := l.Layout(root, Box{Width: 80, Height: 24})
	if err != nil {
		t.Fatalf("Layout returned error: %v", err)
	}
	got := boxes[root]
	if got.Width != 20 || got.Height != 5 {
		t.Errorf("Layout = %+v want Width=20 Height=5", got)
	}
}
