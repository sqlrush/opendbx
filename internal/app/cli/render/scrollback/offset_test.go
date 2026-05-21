// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package scrollback

import (
	"testing"
)

// TestRebuildOffsets_SentinelInvariant verifies the spec-1.5 D-2 sentinel
// prefix-sum invariant: len(offsets)==len(heights)+1, offsets[0]=0,
// offsets[i]=offsets[i-1]+heights[i-1], totalHeight=offsets[len-1].
func TestRebuildOffsets_SentinelInvariant(t *testing.T) {
	cases := []struct {
		name    string
		heights []int
		want    []int
	}{
		{"empty", []int{}, []int{0}},
		{"single", []int{5}, []int{0, 5}},
		{"three blocks h=[2,3,4]", []int{2, 3, 4}, []int{0, 2, 5, 9}},
		{"five blocks unit heights", []int{1, 1, 1, 1, 1}, []int{0, 1, 2, 3, 4, 5}},
		{"zero-height blocks allowed", []int{2, 0, 3}, []int{0, 2, 2, 5}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := rebuildOffsets(tc.heights)
			if len(got) != len(tc.heights)+1 {
				t.Fatalf("len(offsets)=%d, want %d (heights+1)", len(got), len(tc.heights)+1)
			}
			if got[0] != 0 {
				t.Fatalf("offsets[0]=%d, want 0 (sentinel head)", got[0])
			}
			if !equalInts(got, tc.want) {
				t.Fatalf("rebuildOffsets(%v) = %v, want %v", tc.heights, got, tc.want)
			}
			// Invariant: offsets[i] == offsets[i-1] + heights[i-1] for i>=1.
			for i := 1; i < len(got); i++ {
				if got[i] != got[i-1]+tc.heights[i-1] {
					t.Fatalf("invariant break at i=%d: offsets[%d]=%d, want offsets[%d]+heights[%d]=%d+%d=%d",
						i, i, got[i], i-1, i-1, got[i-1], tc.heights[i-1], got[i-1]+tc.heights[i-1])
				}
			}
		})
	}
}

// TestTotalHeight verifies totalHeight returns the last sentinel value
// (sum of all heights). Empty case must return 0.
func TestTotalHeight(t *testing.T) {
	cases := []struct {
		name    string
		offsets []int
		want    int
	}{
		{"empty (just sentinel)", []int{0}, 0},
		{"single block h=5", []int{0, 5}, 5},
		{"three blocks", []int{0, 2, 5, 9}, 9},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := totalHeight(tc.offsets); got != tc.want {
				t.Fatalf("totalHeight(%v) = %d, want %d", tc.offsets, got, tc.want)
			}
		})
	}
}

// TestFindBlockAt_Boundary verifies findBlockAt across boundaries
// (y=0, y=middle, y=block-end-1, y at totalHeight, y past totalHeight).
// Per spec-1.5 R2 D6: returns single int (block index containing line y);
// subLineOffset = y - offsets[idx] is caller-derivable.
func TestFindBlockAt_Boundary(t *testing.T) {
	// Heights: [2, 3, 4] → offsets [0, 2, 5, 9], totalHeight=9.
	// y=0,1 → block 0; y=2,3,4 → block 1; y=5..8 → block 2.
	offsets := []int{0, 2, 5, 9}
	cases := []struct {
		name string
		y    int
		want int
	}{
		{"y=0 first line of block 0", 0, 0},
		{"y=1 last line of block 0", 1, 0},
		{"y=2 first line of block 1", 2, 1},
		{"y=4 last line of block 1", 4, 1},
		{"y=5 first line of block 2", 5, 2},
		{"y=8 last line of block 2", 8, 2},
		{"y=9 past totalHeight clamps to last", 9, 2},
		{"y=100 way past totalHeight clamps to last", 100, 2},
		{"y=-1 negative clamps to 0", -1, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := findBlockAt(offsets, tc.y); got != tc.want {
				t.Fatalf("findBlockAt(offsets, y=%d) = %d, want %d", tc.y, got, tc.want)
			}
		})
	}
}

// TestFindBlockAt_EmptyScrollback verifies findBlockAt on empty offsets
// (just sentinel head [0]) returns 0 — caller must check len(nodes)==0
// before using the result.
func TestFindBlockAt_EmptyScrollback(t *testing.T) {
	offsets := []int{0}
	if got := findBlockAt(offsets, 0); got != 0 {
		t.Fatalf("findBlockAt([0], 0) = %d, want 0 (caller responsibility to gate on len(nodes))", got)
	}
}

// TestRebuildOffsets_AfterDropAllocates verifies rebuildOffsets returns a
// fresh slice (caller assigns; no in-place dependency on input).
func TestRebuildOffsets_AfterDropAllocates(t *testing.T) {
	heights := []int{1, 2, 3}
	first := rebuildOffsets(heights)
	heights = heights[1:] // simulate drop oldest
	second := rebuildOffsets(heights)
	if len(first) == len(second) {
		t.Fatalf("expected different lens after drop: first=%d second=%d", len(first), len(second))
	}
	if second[0] != 0 || second[len(second)-1] != 5 {
		t.Fatalf("after drop heights=[2,3], offsets should be [0,2,5]; got %v", second)
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
