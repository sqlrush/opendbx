// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package layout provides Yoga-like flex box primitives. spec-0.13 D-1
// ships the interface skeleton only; the actual Yoga-like algorithm is
// spec-1.1-flex-layout (gated by spec-0.12.5-flex-spike risk validation).
//
// DAG position: render/layout is index 4 (depends on render/width).
//
// Design: spec-0.13-render-engine-skeleton § 2.1 (D-1)

package layout

// Box is the laid-out rectangle for a node.
type Box struct {
	X, Y          int
	Width, Height int
}

// Node is the input tree node. Concrete types come in spec-1.1.
type Node interface {
	Intrinsic() (width, height int) // intrinsic preferred size
}

// Layouter computes Box positions for a tree of Nodes.
type Layouter interface {
	Layout(root Node, viewport Box) Box
}
