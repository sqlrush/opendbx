// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package layout

import (
	"path/filepath"
	"testing"
)

// BenchmarkLayout1Node baselines single-node layout (degenerate case).
func BenchmarkLayout1Node(b *testing.B) {
	root := &FlexNode{Measure: func() (int, int) { return 80, 24 }}
	viewport := Box{Width: 80, Height: 24}
	l := NewFlexLayouter()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = l.Layout(root, viewport)
	}
}

// BenchmarkLayout10Nodes baselines a 10-node flat row.
func BenchmarkLayout10Nodes(b *testing.B) { benchFlatRow(b, 10) }

// BenchmarkLayout100Nodes baselines a 100-node flat row.
func BenchmarkLayout100Nodes(b *testing.B) { benchFlatRow(b, 100) }

// BenchmarkLayout1000Nodes is the spec-0.12.5 § 2.2 outcome-A gate:
// < 5ms/op required to keep the self-implemented algorithm viable.
func BenchmarkLayout1000Nodes(b *testing.B) { benchFlatRow(b, 1000) }

// BenchmarkLayoutDeepNested baselines a 1000-node column-of-rows tree.
func BenchmarkLayoutDeepNested(b *testing.B) {
	root := buildNested(100, 10)
	viewport := Box{Width: 200, Height: 100}
	l := NewFlexLayouter()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = l.Layout(root, viewport)
	}
}

// BenchmarkLayout5CCSamples runs the locked 5 fixture set once per
// iteration; per-sample budget is < 1ms/op so all 5 < 5ms aggregate.
func BenchmarkLayout5CCSamples(b *testing.B) {
	type loaded struct {
		root *FlexNode
		vp   Box
	}
	loads := make([]loaded, 0, len(sampleNames))
	for _, name := range sampleNames {
		fx, err := loadFixture(filepath.Join("testdata", "cc-samples", name+".json"))
		if err != nil {
			b.Fatalf("load fixture %s: %v", name, err)
		}
		root, _, err := fx.Root.buildTree()
		if err != nil {
			b.Fatalf("build tree %s: %v", name, err)
		}
		loads = append(loads, loaded{root: root, vp: fx.Viewport.viewportBox()})
	}
	l := NewFlexLayouter()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, ld := range loads {
			_, _ = l.Layout(ld.root, ld.vp)
		}
	}
}

// benchFlatRow builds a single Row container with n leaf children
// (each Intrinsic 1×1, grow=1), lays out into a 4*n viewport, b.N times.
func benchFlatRow(b *testing.B, n int) {
	b.Helper()
	children := make([]*FlexNode, n)
	for i := range children {
		children[i] = &FlexNode{Grow: 1, Measure: func() (int, int) { return 1, 1 }}
	}
	root := &FlexNode{Direction: Row, Children: children}
	viewport := Box{Width: 4 * n, Height: 1}
	l := NewFlexLayouter()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = l.Layout(root, viewport)
	}
}

// buildNested constructs a 2-level tree: a Column container with rows
// children, each row containing perRow leaf children. Total nodes:
// 1 + rows + rows*perRow.
func buildNested(rows, perRow int) *FlexNode {
	rootChildren := make([]*FlexNode, rows)
	for i := range rootChildren {
		leaves := make([]*FlexNode, perRow)
		for j := range leaves {
			leaves[j] = &FlexNode{Grow: 1, Measure: func() (int, int) { return 1, 1 }}
		}
		rootChildren[i] = &FlexNode{Direction: Row, BasisMode: BasisFixed, Basis: 1, Children: leaves}
	}
	return &FlexNode{Direction: Column, Children: rootChildren}
}
