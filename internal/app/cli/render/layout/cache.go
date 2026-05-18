// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package layout

// measureCache holds per-Layout-call leaf intrinsic measurements plus
// a re-entry guard set for cycle detection (R2-7 call-count contract
// + R2-8 measurement callback re-entry detection).
//
// Lifetime: a fresh measureCache is constructed at the start of every
// Layout() call and discarded when the call returns. Cross-frame
// caching is explicitly out of scope for spec-1.1 — spec-1.4
// scheduler frame loop owns that contract.
//
// Concurrency: not safe for concurrent use; Layout() is single-threaded
// per call (see Layouter godoc). One measureCache instance is
// confined to the goroutine running its owning Layout() call.
type measureCache struct {
	// results memoizes each leaf's Intrinsic() output. A second call
	// for the same Node returns the stored values WITHOUT re-invoking
	// the user-provided callback (R2-7 single-call-per-leaf contract).
	results map[*FlexNode]intrinsicResult
	// inProgress marks Nodes currently being measured. A re-entry
	// (A.Intrinsic() invokes B.Intrinsic() invokes A.Intrinsic())
	// trips this guard and returns ErrLayoutCycle (R2-8 measurement
	// callback re-entry only; child graph cycles are caller
	// responsibility per the Layouter contract).
	inProgress map[*FlexNode]bool
}

// intrinsicResult is the cached (w, h) pair for a single leaf.
type intrinsicResult struct {
	w, h int
}

func newMeasureCache() *measureCache {
	return &measureCache{
		results:    map[*FlexNode]intrinsicResult{},
		inProgress: map[*FlexNode]bool{},
	}
}

// measure returns the leaf's intrinsic (w, h). On a cache hit, the
// stored values are returned without re-invoking node.Measure(). On a
// cache miss, node.Measure() is invoked exactly once and the result is
// stored; if the callback re-enters this measureCache.measure for the
// same node (directly or transitively), ErrLayoutCycle is returned.
//
// Callers MUST only call measure on leaf nodes whose Measure callback
// is non-nil; the algorithm's intrinsicPass guards this precondition.
func (c *measureCache) measure(node *FlexNode) (int, int, error) {
	if r, ok := c.results[node]; ok {
		return r.w, r.h, nil
	}
	if c.inProgress[node] {
		return 0, 0, ErrLayoutCycle
	}
	c.inProgress[node] = true
	w, h := node.Measure()
	delete(c.inProgress, node)
	c.results[node] = intrinsicResult{w: w, h: h}
	return w, h, nil
}
