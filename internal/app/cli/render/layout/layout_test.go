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

func (f fakeNode) Intrinsic() (int, int) { return f.w, f.h }

type fakeLayouter struct{}

func (fakeLayouter) Layout(root Node, viewport Box) Box {
	w, h := root.Intrinsic()
	return Box{X: 0, Y: 0, Width: w, Height: h}
}

func TestLayouter_InterfaceContract(t *testing.T) {
	t.Parallel()
	var l Layouter = fakeLayouter{}
	got := l.Layout(fakeNode{w: 20, h: 5}, Box{Width: 80, Height: 24})
	if got.Width != 20 || got.Height != 5 {
		t.Errorf("Layout = %+v want Width=20 Height=5", got)
	}
}
