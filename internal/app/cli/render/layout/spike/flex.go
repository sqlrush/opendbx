// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

//go:build spike
// +build spike

package spike

import (
	"github.com/sqlrush/opendbx/internal/app/cli/render/layout"
)

// Direction is the main-axis direction of a flex container.
type Direction uint8

const (
	// Row lays children horizontally; main axis = width.
	Row Direction = iota
	// Column lays children vertically; main axis = height.
	Column
)

// BasisMode distinguishes a fixed basis value from "auto" (use intrinsic).
type BasisMode uint8

const (
	// BasisAuto: node's basis on the main axis = its intrinsic size
	// projected onto the parent's main axis.
	BasisAuto BasisMode = iota
	// BasisFixed: node uses Basis (cell count) as its starting main-axis size.
	BasisFixed
)

// IntrinsicFn returns the natural (preferred) size of a leaf node in (w, h)
// cells before any grow/shrink redistribution. Called at most once per
// Layout call.
type IntrinsicFn func() (width, height int)

// FlexNode is one node in the input tree fed to Layout.
//
// A container's Direction defines how its OWN children are laid out
// (main vs cross). A leaf node's Direction is ignored; only Intrinsic
// matters at leaves.
//
// Grow / Shrink / Basis apply along the PARENT's main axis. Cross-axis
// sizing is always "stretch to fill parent's cross" in this spike
// (align-items / justify-content are out of scope per spec-0.12.5 § 1.2 ❌-2).
type FlexNode struct {
	Direction Direction
	Grow      float64
	Shrink    float64
	Basis     int
	BasisMode BasisMode
	Intrinsic IntrinsicFn
	Children  []*FlexNode
}

// meta is internal accounting attached to each node during a single
// Layout pass. Both intrinsic and final sizes are stored as 2D (w, h)
// in viewport coordinates; main/cross projection happens at use site.
type meta struct {
	intrinsicW int
	intrinsicH int
	width      int
	height     int
	x, y       int
	parent     *FlexNode
}

// Layout runs the 4-phase Yoga-subset algorithm and returns the absolute
// Box (x, y, w, h) of every node in the tree, keyed by node pointer.
//
// 4 phases:
//   - Phase A (intrinsic): post-order DFS. Each leaf measures via Intrinsic();
//     each container's intrinsic (W, H) is derived from children projected
//     onto the container's own Direction.
//   - Phase B (basis): per-child effectiveBasis along PARENT's main axis
//     = Basis when BasisFixed, else child's intrinsic projected onto
//     parent's main axis.
//   - Phase C (grow/shrink): distribute the difference between parent's
//     main size and sum(child.effectiveBasis) using grow weights (positive
//     remainder) or shrink weights (negative remainder). Output is each
//     child's final width + height in viewport coordinates.
//   - Phase D (position): pre-order DFS. Children laid out sequentially
//     along parent's main axis starting from the parent's (x, y).
//
// Complexity: O(N) total node visits across all phases.
func Layout(root *FlexNode, viewport layout.Box) map[*FlexNode]layout.Box {
	if root == nil {
		return map[*FlexNode]layout.Box{}
	}
	metas := make(map[*FlexNode]*meta)
	intrinsicPass(root, nil, metas)

	rm := metas[root]
	rm.width = viewport.Width
	rm.height = viewport.Height
	rm.x = viewport.X
	rm.y = viewport.Y

	distributePass(root, metas)
	positionPass(root, metas)

	result := make(map[*FlexNode]layout.Box, len(metas))
	for node, m := range metas {
		result[node] = layout.Box{X: m.x, Y: m.y, Width: m.width, Height: m.height}
	}
	return result
}

// intrinsicPass walks post-order, filling meta.intrinsicW / intrinsicH
// for every node. Container intrinsic = sum along its own main axis,
// max along its own cross axis.
func intrinsicPass(node *FlexNode, parent *FlexNode, metas map[*FlexNode]*meta) {
	m := &meta{parent: parent}
	metas[node] = m
	if len(node.Children) == 0 {
		if node.Intrinsic != nil {
			m.intrinsicW, m.intrinsicH = node.Intrinsic()
		}
		return
	}
	sumMain := 0
	maxCross := 0
	for _, child := range node.Children {
		intrinsicPass(child, node, metas)
		cm := metas[child]
		cMain := mainOf(node.Direction, cm.intrinsicW, cm.intrinsicH)
		cCross := crossOf(node.Direction, cm.intrinsicW, cm.intrinsicH)
		sumMain += cMain
		if cCross > maxCross {
			maxCross = cCross
		}
	}
	m.intrinsicW = widthOf(node.Direction, sumMain, maxCross)
	m.intrinsicH = heightOf(node.Direction, sumMain, maxCross)
}

// distributePass walks top-down. For each container, compute each child's
// final width + height using Phase B (basis) + Phase C (grow/shrink).
// Recurses into each child as a container of its own children.
func distributePass(node *FlexNode, metas map[*FlexNode]*meta) {
	if len(node.Children) == 0 {
		return
	}
	pm := metas[node]
	parentMain := mainOf(node.Direction, pm.width, pm.height)
	parentCross := crossOf(node.Direction, pm.width, pm.height)

	// Phase B: effectiveBasis along parent's main axis.
	bases := make([]int, len(node.Children))
	totalBasis := 0
	for i, child := range node.Children {
		cm := metas[child]
		var b int
		if child.BasisMode == BasisFixed {
			b = child.Basis
		} else {
			b = mainOf(node.Direction, cm.intrinsicW, cm.intrinsicH)
		}
		bases[i] = b
		totalBasis += b
	}

	// Phase C: distribute remainder.
	mainSizes := make([]int, len(node.Children))
	copy(mainSizes, bases)
	remainder := parentMain - totalBasis
	switch {
	case remainder > 0:
		totalGrow := 0.0
		for _, c := range node.Children {
			totalGrow += c.Grow
		}
		if totalGrow > 0 {
			used := 0
			for i, child := range node.Children {
				share := int(float64(remainder) * (child.Grow / totalGrow))
				mainSizes[i] = bases[i] + share
				used += mainSizes[i]
			}
			leftover := parentMain - used
			if leftover != 0 {
				if idx := largestGrowIdx(node.Children); idx >= 0 {
					mainSizes[idx] += leftover
				}
			}
		}
	case remainder < 0:
		overflow := -remainder
		totalShrink := 0.0
		for _, c := range node.Children {
			totalShrink += c.Shrink
		}
		if totalShrink > 0 {
			for i, child := range node.Children {
				cut := int(float64(overflow) * (child.Shrink / totalShrink))
				v := bases[i] - cut
				if v < 0 {
					v = 0
				}
				mainSizes[i] = v
			}
		}
	}

	// Write final width / height into each child meta, then recurse.
	for i, child := range node.Children {
		cm := metas[child]
		cm.width = widthOf(node.Direction, mainSizes[i], parentCross)
		cm.height = heightOf(node.Direction, mainSizes[i], parentCross)
		distributePass(child, metas)
	}
}

// positionPass walks pre-order. Children are placed sequentially along
// the parent's main axis; cross coordinate matches parent's cross origin.
func positionPass(node *FlexNode, metas map[*FlexNode]*meta) {
	if len(node.Children) == 0 {
		return
	}
	pm := metas[node]
	// (x, y) cursor in viewport coordinates. For Row: advance x; for
	// Column: advance y. Cross stays at parent's cross origin.
	cursorX := pm.x
	cursorY := pm.y
	for _, child := range node.Children {
		cm := metas[child]
		cm.x = cursorX
		cm.y = cursorY
		if node.Direction == Row {
			cursorX += cm.width
		} else {
			cursorY += cm.height
		}
		positionPass(child, metas)
	}
}

// --- axis helpers ----------------------------------------------------

func mainOf(dir Direction, w, h int) int {
	if dir == Row {
		return w
	}
	return h
}

func crossOf(dir Direction, w, h int) int {
	if dir == Row {
		return h
	}
	return w
}

func widthOf(parentDir Direction, mainSize, crossSize int) int {
	if parentDir == Row {
		return mainSize
	}
	return crossSize
}

func heightOf(parentDir Direction, mainSize, crossSize int) int {
	if parentDir == Row {
		return crossSize
	}
	return mainSize
}

func largestGrowIdx(children []*FlexNode) int {
	best := -1
	bestGrow := -1.0
	for i, c := range children {
		if c.Grow > bestGrow {
			best = i
			bestGrow = c.Grow
		}
	}
	return best
}
