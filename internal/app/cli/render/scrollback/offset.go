// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File offset.go — sentinel prefix-sum offset table + O(log N) binary
// search block lookup. spec-1.5 D-2 (R2 D2 + D6 errata).
//
// Invariant (R2 D2): for heights []int of length N,
//
//	len(offsets) == N + 1
//	offsets[0]   == 0                       (sentinel head)
//	offsets[i]   == offsets[i-1] + heights[i-1]   for 1 <= i <= N
//	totalHeight  == offsets[N]              (sentinel tail)
//
// The sentinel head + tail simplifies edge cases (empty scrollback is
// offsets=[0]; totalHeight=0) and lets findBlockAt clamp purely on
// sort.SearchInts result without separate empty handling.

package scrollback

import "sort"

// rebuildOffsets builds a fresh sentinel prefix-sum offsets slice from
// heights. O(N). The returned slice has length len(heights)+1; offsets[0]=0
// and offsets[len-1]=sum(heights). Callers assign the result; the function
// allocates a new slice each call (caller may pre-allocate via dst pattern
// if profiling shows allocator pressure — not done in MVP per R-1).
//
// Empty heights produces offsets=[0] (just the sentinel head; totalHeight=0).
func rebuildOffsets(heights []int) []int {
	out := make([]int, len(heights)+1)
	// out[0] = 0 implicit
	for i, h := range heights {
		out[i+1] = out[i] + h
	}
	return out
}

// totalHeight returns the cumulative height of all blocks, which under
// the sentinel invariant equals offsets[len(offsets)-1]. Empty scrollback
// (offsets=[0]) returns 0. Caller must hold any required lock externally.
func totalHeight(offsets []int) int {
	return offsets[len(offsets)-1]
}

// findBlockAt returns the index of the block containing line y. Uses
// sort.SearchInts on the offsets table: SearchInts returns the smallest
// index i such that offsets[i] >= target. With target = y+1, that yields
// the first sentinel position strictly greater than y; subtract 1 to get
// the block whose [offsets[block], offsets[block+1]) range contains y.
//
// Edge cases (R2 D6 — single int return):
//   - y < 0: clamps to 0 (caller should pre-clamp scrollY).
//   - y >= totalHeight: clamps to last block (len(offsets)-2).
//   - Empty scrollback (offsets=[0]): returns 0 (caller must gate on
//     len(nodes)==0 separately).
//
// Caller derives subLineOffset as `y - offsets[block]` when needed.
func findBlockAt(offsets []int, y int) int {
	if y < 0 {
		return 0
	}
	idx := sort.SearchInts(offsets, y+1) - 1
	if idx < 0 {
		return 0
	}
	last := len(offsets) - 2 // last block index = len(offsets) - 2 (offsets has N+1 entries for N blocks)
	if idx > last {
		if last < 0 {
			return 0
		}
		return last
	}
	return idx
}
