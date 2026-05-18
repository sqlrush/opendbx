// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package layout provides Yoga-like flex box primitives — direction
// (row/column), grow, shrink, basis (px/auto), justify-content, and
// align-items (the 6-dim minimal Yoga subset; AD-002).
//
// spec-0.13 D-1 shipped the Box / Node / Layouter interface skeleton;
// spec-1.1 fills the production-grade Yoga-like algorithm behind it
// (4-phase DFS: intrinsic → basis → grow/shrink → position) including
// measure cache + measurement callback re-entry cycle detection +
// RENDER.* error 三件套.
//
// Layouter signature was upgraded via spec-0.13 § 11.5 inline errata
// (spec-1.1 D-6): `Layout(root Node, viewport Box) Box` →
// `Layout(root Node, viewport Box) (map[Node]Box, error)` to let
// downstream callers (spec-1.2 cell-grid-buffer, spec-1.7 block-interface)
// retrieve every child's placement, and to surface invalid input +
// cycle errors as recoverable values rather than panics.
//
// DAG position: render/layout is index 4 (depends on render/width).
//
// Design: spec-0.13 D-1 (skeleton) + spec-1.1 (algorithm)
package layout

// Box is the laid-out rectangle for a node in absolute viewport coordinates.
type Box struct {
	X, Y          int
	Width, Height int
}

// Node is the input tree node. spec-1.1 ships *FlexNode as the canonical
// pointer-backed implementation; any Node must be Go-comparable
// (`==` safely defined) because it is used as a map key in the
// `map[Node]Box` return value (R2-1 Node comparability contract).
//
// Non-comparable types (slice / map / func fields without indirection)
// will panic at runtime when Layout stores them in the result map.
// Implementations are STRONGLY recommended to be pointer-backed structs
// (e.g. *FlexNode) so equality is identity, never value-equality.
type Node interface {
	Intrinsic() (width, height int) // intrinsic preferred size in cells
}

// Layouter computes the absolute Box of every node in a tree.
//
// Layout returns a placement map for all nodes reachable from root.
// boxes[root] is the root Box (equivalent to the spec-0.13 single-Box
// return); boxes[child] gives every child's absolute position. The
// returned map is safe to range; iteration order is not stable.
//
// Returns an error from the RENDER.* family (see errors.go) for:
//   - viewport.Width ≤ 0 or viewport.Height ≤ 0 (INVALID_DIMENSION)
//   - any negative Grow / Shrink / Basis / Intrinsic value (INVALID_DIMENSION)
//   - container with more than 1000 children (INVALID_DIMENSION, R2-9 cap)
//   - arithmetic overflow during sizing (INVALID_DIMENSION, R2-9 guard)
//   - Intrinsic() callback re-entering layout on the same Node
//     (LAYOUT_CYCLE; measurement callback re-entry only — child graph
//     cycles are caller responsibility per R2-8)
//
// Node MUST be Go-comparable; non-comparable Nodes panic at the map
// store site (Go map key requirement; not a layout-side error path).
//
// Concurrency: Layouter is stateless and can be invoked from any
// goroutine; the input tree must NOT mutate during a Layout() call;
// Intrinsic() callbacks must be caller-synchronized.
type Layouter interface {
	Layout(root Node, viewport Box) (map[Node]Box, error)
}
