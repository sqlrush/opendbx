// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package layout

import (
	"math"
	"sync"
)

// MaxChildren is the per-container child count cap. Containers with more
// children than this trigger ErrInvalidDimension (R2-9 fixture size cap).
// 1000 is well above any plausible CC UI block layout (CC scrollback +
// status-line + tool-call-panel rarely exceeds ~30 children at any depth)
// while keeping the per-Layout-call allocation bounded.
const MaxChildren = 1000

// Direction is the main-axis direction of a flex container.
type Direction uint8

const (
	// Row lays children horizontally; main axis = width. CC Ink default
	// (`~/claude-code-source-code/src/ink/components/Box.tsx` line 109:
	// `flexDirection = t3 === undefined ? "row" : t3`). Note this differs
	// from the native yoga library default (column) — opendbx aligns to
	// CC Ink, not raw yoga, per CLAUDE.md principle 1.
	Row Direction = iota
	// Column lays children vertically; main axis = height.
	Column
)

// BasisMode distinguishes a fixed basis value from "auto" (use intrinsic).
type BasisMode uint8

const (
	// BasisAuto uses the child's intrinsic size projected onto the parent's
	// main axis.
	BasisAuto BasisMode = iota
	// BasisFixed uses Basis (cell count) as the child's starting main-axis
	// size, ignoring intrinsic for sizing (intrinsic still feeds the
	// parent's intrinsic accounting in nested containers).
	BasisFixed
)

// IntrinsicFn returns the natural (preferred) size of a leaf node in
// (w, h) cells before any grow/shrink redistribution. Called at most
// once per leaf per Layout call (R2-7 call-count contract; enforced
// internally via the per-Layout measure cache + re-entry guard).
//
// The function MUST NOT recursively invoke Layout on the same Node via
// the same Layouter instance or call back into another Node's IntrinsicFn
// that transitively reaches the same Node — that pattern is the
// measurement-callback re-entry cycle detected by ErrLayoutCycle (R2-8).
type IntrinsicFn func() (width, height int)

// FlexNode is one node in the input tree fed to Layout.
//
// A container's Direction defines how its OWN children are laid out
// (main vs cross). A leaf node's Direction is ignored; only Intrinsic
// matters at leaves.
//
// Grow / Shrink / Basis apply along the PARENT's main axis.
//
// Defaults (CC Ink Box.tsx verified L109-111):
//   - Direction = Row
//   - Grow     = 0
//   - Shrink   = 1
//   - Justify  = JustifyStart
//   - Align    = AlignStretch
//   - Basis    = 0 (BasisMode = BasisAuto)
//
// Zero-value FlexNode is a valid leaf with intrinsic 0x0 and CC-Ink-aligned
// defaults; but Shrink defaults to 1 by CC Ink, NOT by Go zero-value (Go
// zero is 0). Construct FlexNodes via NewFlexNode for safe defaults; raw
// literal `&FlexNode{...}` callers must set Shrink=1 explicitly to match
// CC Ink semantics. The algorithm treats Shrink=0 as "do not shrink"
// per the W3C standard, so the zero-value behavior is well-defined —
// it just differs from CC Ink defaults.
type FlexNode struct {
	Direction Direction
	Grow      float64
	Shrink    float64
	Basis     int
	BasisMode BasisMode
	Justify   Justify
	Align     Align
	// Measure is the IntrinsicFn callback for leaf nodes. nil for
	// containers (intrinsic computed from children). Named Measure (not
	// `Intrinsic`) so it does not name-clash with the Node.Intrinsic()
	// method satisfied by *FlexNode.
	Measure  IntrinsicFn
	Children []*FlexNode
}

// NewFlexNode constructs a FlexNode pre-populated with CC-Ink-aligned
// defaults (Direction=Row, Grow=0, Shrink=1, Justify=Start,
// Align=Stretch, Basis=0/Auto). Callers can override any field after
// construction.
func NewFlexNode() *FlexNode {
	return &FlexNode{
		Direction: Row,
		Grow:      0,
		Shrink:    1,
		BasisMode: BasisAuto,
		Justify:   JustifyStart,
		Align:     AlignStretch,
	}
}

// Intrinsic on *FlexNode satisfies the Node interface. Leaf nodes return
// their Measure callback result (or 0x0 if unset); container nodes
// return 0x0 here — the algorithm computes container intrinsics
// internally from children. Callers that bind a non-flex Node type to
// Layouter can implement Intrinsic directly without using FlexNode.
func (n *FlexNode) Intrinsic() (int, int) {
	if n == nil || n.Measure == nil {
		return 0, 0
	}
	return n.Measure()
}

// flexLayouter is the production Layouter implementation backed by the
// 4-phase DFS algorithm promoted from the spec-0.12.5 spike (outcome A).
//
// It only carries a same-node re-entry guard; per-call layout state lives
// entirely on the stack of Layout / layoutTree (the metas map + measure
// cache). The guard catches measurement callbacks that call Layout again
// on the same active node through the same Layouter instance.
type flexLayouter struct {
	mu     sync.Mutex
	active map[*FlexNode]bool
}

// NewFlexLayouter returns a production-grade Yoga-like flex Layouter
// implementing the spec-1.1 6-dim minimal subset (direction, grow,
// shrink, basis, justify, align).
func NewFlexLayouter() Layouter {
	return &flexLayouter{}
}

// Layout runs the 4-phase Yoga-subset algorithm and returns the
// absolute Box (x, y, w, h) of every node reachable from root.
//
// The 4 phases are:
//
//   - Phase A (intrinsic, post-order DFS): each leaf measures via its
//     Intrinsic() callback; each container's intrinsic is derived from
//     its children, projected onto the container's own Direction.
//
//   - Phase B (basis, pre-order): per-child effectiveBasis along the
//     PARENT's main axis = Basis when BasisFixed, else the child's
//     intrinsic projected onto the parent's main axis.
//
//   - Phase C (grow/shrink): distribute the difference between the
//     parent's main size and sum(effectiveBasis) using grow weights
//     (positive remainder) or shrink weights (negative remainder).
//     Output is each child's final width + height.
//
//   - Phase D (position, pre-order): main-axis offsets per Justify;
//     cross-axis size + offset per Align. Children are laid out
//     sequentially starting from the parent's (x, y) plus the Justify
//     leading-space.
//
// Complexity: O(N) total node visits across all phases (N = tree size).
//
// Errors: see Layouter interface godoc in layout.go.
func (l *flexLayouter) Layout(root Node, viewport Box) (map[Node]Box, error) {
	if viewport.Width <= 0 || viewport.Height <= 0 {
		return nil, ErrInvalidDimension
	}
	if viewport.Width > math.MaxInt32 || viewport.Height > math.MaxInt32 {
		return nil, ErrInvalidDimension
	}
	if root == nil {
		return map[Node]Box{}, nil
	}
	fr, ok := root.(*FlexNode)
	if !ok {
		// Non-FlexNode Node: treat as a leaf and return its intrinsic
		// clamped to viewport. This preserves the Layouter interface
		// for callers that bring their own Node implementation.
		w, h := root.Intrinsic()
		if w < 0 || h < 0 {
			return nil, ErrInvalidDimension
		}
		return map[Node]Box{root: {
			X:      viewport.X,
			Y:      viewport.Y,
			Width:  clampToViewport(w, viewport.Width),
			Height: clampToViewport(h, viewport.Height),
		}}, nil
	}
	if !l.enterNode(fr) {
		return nil, ErrLayoutCycle
	}
	defer l.leaveNode(fr)

	if err := validateTree(fr); err != nil {
		return nil, err
	}

	metas := make(map[*FlexNode]*meta)
	cache := newMeasureCache()
	if err := l.intrinsicPass(fr, nil, metas, cache); err != nil {
		return nil, err
	}

	rm := metas[fr]
	rm.width = viewport.Width
	rm.height = viewport.Height
	rm.x = viewport.X
	rm.y = viewport.Y

	if err := distributePass(fr, metas); err != nil {
		return nil, err
	}
	positionPass(fr, metas)

	result := make(map[Node]Box, len(metas))
	for node, m := range metas {
		result[node] = Box{X: m.x, Y: m.y, Width: m.width, Height: m.height}
	}
	return result, nil
}

func (l *flexLayouter) enterNode(node *FlexNode) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.active == nil {
		l.active = make(map[*FlexNode]bool)
	}
	if l.active[node] {
		return false
	}
	l.active[node] = true
	return true
}

func (l *flexLayouter) leaveNode(node *FlexNode) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.active, node)
}

// meta is internal accounting attached to each node during a single
// Layout pass. Both intrinsic and final sizes are stored as 2D (w, h)
// in viewport coordinates; main/cross projection happens at use sites.
type meta struct {
	intrinsicW int
	intrinsicH int
	width      int
	height     int
	x, y       int
	parent     *FlexNode
}

// validateTree walks the tree once before phase A to check structural
// invariants (no negative grow/shrink/basis; child count ≤ MaxChildren;
// no duplicate node references).
// Returning ErrInvalidDimension here fails fast with a single error
// before any allocation in metas.
func validateTree(node *FlexNode) error {
	return validateTreeNode(node, make(map[*FlexNode]bool))
}

func validateTreeNode(node *FlexNode, seen map[*FlexNode]bool) error {
	if node == nil {
		return nil
	}
	if seen[node] {
		return ErrInvalidDimension
	}
	seen[node] = true
	if node.Grow < 0 || node.Shrink < 0 {
		return ErrInvalidDimension
	}
	if node.BasisMode == BasisFixed && node.Basis < 0 {
		return ErrInvalidDimension
	}
	if len(node.Children) > MaxChildren {
		return ErrInvalidDimension
	}
	for _, c := range node.Children {
		if c == nil {
			return ErrInvalidDimension
		}
		if err := validateTreeNode(c, seen); err != nil {
			return err
		}
	}
	return nil
}

// intrinsicPass walks post-order, filling meta.intrinsicW / intrinsicH
// for every node. Container intrinsic = sum along its own main axis,
// max along its own cross axis. Leaf intrinsic comes from the measure
// cache (one Intrinsic() call per leaf per Layout call; R2-7 + R2-8
// re-entry detection).
func (l *flexLayouter) intrinsicPass(node *FlexNode, parent *FlexNode, metas map[*FlexNode]*meta, cache *measureCache) error {
	m := &meta{parent: parent}
	metas[node] = m
	if len(node.Children) == 0 {
		if node.Measure != nil {
			if parent != nil {
				if !l.enterNode(node) {
					return ErrLayoutCycle
				}
				defer l.leaveNode(node)
			}
			w, h, err := cache.measure(node)
			if err != nil {
				return err
			}
			if w < 0 || h < 0 {
				return ErrInvalidDimension
			}
			m.intrinsicW = w
			m.intrinsicH = h
		}
		return nil
	}
	sumMain := 0
	maxCross := 0
	for _, child := range node.Children {
		if err := l.intrinsicPass(child, node, metas, cache); err != nil {
			return err
		}
		cm := metas[child]
		cMain := mainOf(node.Direction, cm.intrinsicW, cm.intrinsicH)
		cCross := crossOf(node.Direction, cm.intrinsicW, cm.intrinsicH)
		// Overflow guard (R2-9): cumulative main-axis sum must fit int32.
		if sumMain > math.MaxInt32-cMain {
			return ErrInvalidDimension
		}
		sumMain += cMain
		if cCross > maxCross {
			maxCross = cCross
		}
	}
	m.intrinsicW = widthOf(node.Direction, sumMain, maxCross)
	m.intrinsicH = heightOf(node.Direction, sumMain, maxCross)
	return nil
}

// distributePass walks top-down. For each container, compute each
// child's main-axis size (Phase B basis + Phase C grow/shrink); cross
// sizing per Align is deferred to positionPass which has both axes
// available. Recurses into each child as a potential sub-container.
func distributePass(node *FlexNode, metas map[*FlexNode]*meta) error {
	if len(node.Children) == 0 {
		return nil
	}
	pm := metas[node]
	parentMain := mainOf(node.Direction, pm.width, pm.height)

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
		// Overflow guard.
		if totalBasis > math.MaxInt32-b {
			return ErrInvalidDimension
		}
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
				if mainSizes[i] > math.MaxInt32 || used > math.MaxInt32-mainSizes[i] {
					return ErrInvalidDimension
				}
				used += mainSizes[i]
			}
			leftover := parentMain - used
			if leftover != 0 {
				idx := largestGrowIdx(node.Children)
				if idx >= 0 {
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
			actualCut := 0
			for i, child := range node.Children {
				cut := int(float64(overflow) * (child.Shrink / totalShrink))
				v := bases[i] - cut
				if v < 0 {
					v = 0
				}
				mainSizes[i] = v
				actualCut += bases[i] - v
			}
			leftover := overflow - actualCut
			for leftover > 0 {
				idx := largestShrinkIdxWithRoom(node.Children, mainSizes)
				if idx < 0 {
					break
				}
				mainSizes[idx]--
				leftover--
			}
		}
	}

	// Write each child's full (width, height) into meta: main axis from
	// distribute, cross axis from applyAlign (so children of nested
	// containers have correct cross size before their own distribute).
	parentCross := crossOf(node.Direction, pm.width, pm.height)
	for i, child := range node.Children {
		cm := metas[child]
		childIntrinsicCross := crossOf(node.Direction, cm.intrinsicW, cm.intrinsicH)
		cross, _ := applyAlign(node.Align, parentCross, childIntrinsicCross)
		if node.Direction == Row {
			cm.width = mainSizes[i]
			cm.height = cross
		} else {
			cm.height = mainSizes[i]
			cm.width = cross
		}
		if err := distributePass(child, metas); err != nil {
			return err
		}
	}
	return nil
}

// positionPass walks pre-order and assigns absolute (x, y) coordinates
// to every child of every container based on:
//   - main-axis: Justify leading-space + inter-child gap
//   - cross-axis: Align cross-offset relative to parent cross origin
//
// child widths / heights were already finalized in distributePass; this
// pass only writes x and y.
func positionPass(node *FlexNode, metas map[*FlexNode]*meta) {
	if len(node.Children) == 0 {
		return
	}
	pm := metas[node]
	parentMain := mainOf(node.Direction, pm.width, pm.height)
	parentCross := crossOf(node.Direction, pm.width, pm.height)
	parentMainOrigin := mainOf(node.Direction, pm.x, pm.y)
	parentCrossOrigin := crossOf(node.Direction, pm.x, pm.y)

	// Main-axis: compute Justify leading-space + inter-child gap.
	totalMain := 0
	for _, child := range node.Children {
		cm := metas[child]
		totalMain += mainOf(node.Direction, cm.width, cm.height)
	}
	leading, gap := applyJustify(node.Justify, parentMain, totalMain, len(node.Children))

	cursor := parentMainOrigin + leading
	for i, child := range node.Children {
		cm := metas[child]

		// Cross-axis offset per Align (cross size already in cm.width/height).
		childCross := crossOf(node.Direction, cm.width, cm.height)
		_, crossOff := applyAlign(node.Align, parentCross, childCross)

		if node.Direction == Row {
			cm.x = cursor
			cm.y = parentCrossOrigin + crossOff
		} else {
			cm.y = cursor
			cm.x = parentCrossOrigin + crossOff
		}

		childMain := mainOf(node.Direction, cm.width, cm.height)
		cursor += childMain
		if i < len(node.Children)-1 {
			cursor += gap
		}
		positionPass(child, metas)
	}
}

// --- axis helpers ---------------------------------------------------

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

func clampToViewport(size, viewportSize int) int {
	if size > viewportSize {
		return viewportSize
	}
	return size
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

// largestShrinkIdxWithRoom picks the child with largest Shrink weight
// among those still having room to shrink (mainSizes[i] > 0). Returns
// -1 if no child can shrink further. Used by Phase C shrink leftover
// correction.
func largestShrinkIdxWithRoom(children []*FlexNode, mainSizes []int) int {
	best := -1
	bestShrink := -1.0
	for i, c := range children {
		if mainSizes[i] <= 0 {
			continue
		}
		if c.Shrink > bestShrink {
			best = i
			bestShrink = c.Shrink
		}
	}
	return best
}
