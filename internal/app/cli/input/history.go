// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package input

// RingCapacity is the unified history ring buffer capacity. CC parity
// (CC src/history.ts uses a similar bounded ring); spec-2.12 will add
// disk persistence on top. Not configurable in spec-1.17 — spec-2.x
// loadUserConfig will overlay if needed.
const RingCapacity = 256

// Ring is the spec-1.17 R2 unified history storage. Pure storage —
// navigation state (current index, draft of fresh input) lives in
// caller Models (e.g. demoapp.Model.historyNav) so that the Buffer
// remains the single source of truth for displayed input (spec-1.16
// R2 C2 ★A absorb; spec-1.17 R2 CRIT-4).
//
// Ownership: non-concurrent — exactly one goroutine accesses an
// instance at a time. Models live on the scheduler main goroutine
// (spec-1.15 R3 single-goroutine ownership of p.model), so Ring fits
// naturally. spec-2.12 disk persistence MUST snapshot Ring on the
// owning goroutine then emit a Cmd; cross-goroutine reads are NOT
// supported.
type Ring struct {
	entries []string // circular buffer; len == RingCapacity once filled
	head    int      // next write position (0..RingCapacity-1)
	size    int      // 0..RingCapacity
}

// NewRing constructs an empty history ring.
func NewRing() *Ring {
	return &Ring{
		entries: make([]string, RingCapacity),
	}
}

// Len returns the current number of stored entries (0..RingCapacity).
func (r *Ring) Len() int {
	return r.size
}

// Push appends an entry to the ring. Eviction is FIFO once size hits
// RingCapacity.
//
// Dedup contract (spec-1.17 R2 D-4): consecutive duplicate entries
// collapse — Push("a"); Push("a") yields size=1. Non-consecutive
// duplicates pass through — Push("a"); Push("b"); Push("a") yields
// size=3. Empty strings are dropped (callers should not push empty
// submits, but defensive in case Enter on empty Buffer reaches here).
func (r *Ring) Push(entry string) {
	if entry == "" {
		return
	}
	if r.size > 0 {
		// Read the most recently written entry (head-1 mod cap).
		last := r.entries[(r.head-1+RingCapacity)%RingCapacity]
		if last == entry {
			return
		}
	}
	r.entries[r.head] = entry
	r.head = (r.head + 1) % RingCapacity
	if r.size < RingCapacity {
		r.size++
	}
}

// At returns the entry at logical index idx (0 == oldest, Len()-1 ==
// newest). Returns ("", false) when idx is out of range.
func (r *Ring) At(idx int) (string, bool) {
	if idx < 0 || idx >= r.size {
		return "", false
	}
	// Oldest position = head - size (mod cap).
	pos := (r.head - r.size + idx + RingCapacity) % RingCapacity
	return r.entries[pos], true
}
