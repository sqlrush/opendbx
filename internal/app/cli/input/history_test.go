// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package input

import (
	"fmt"
	"testing"
)

// TestRing_NewIsEmpty asserts the constructed Ring reports Len 0 and
// At returns ("", false) for any index.
func TestRing_NewIsEmpty(t *testing.T) {
	t.Parallel()
	r := NewRing()
	if r.Len() != 0 {
		t.Errorf("Len() = %d; want 0", r.Len())
	}
	if s, ok := r.At(0); ok || s != "" {
		t.Errorf("At(0) on empty = (%q, %v); want (\"\", false)", s, ok)
	}
}

// TestRing_Push covers happy-path Push and the At ordering invariant
// (0 = oldest, Len()-1 = newest).
func TestRing_Push(t *testing.T) {
	t.Parallel()
	r := NewRing()
	r.Push("a")
	r.Push("b")
	r.Push("c")
	if r.Len() != 3 {
		t.Errorf("Len() = %d; want 3", r.Len())
	}
	if s, _ := r.At(0); s != "a" {
		t.Errorf("At(0) = %q; want \"a\" (oldest)", s)
	}
	if s, _ := r.At(2); s != "c" {
		t.Errorf("At(2) = %q; want \"c\" (newest)", s)
	}
}

// TestRing_PushEmpty asserts empty strings are dropped (defensive vs
// callers that hit Enter on an empty buffer).
func TestRing_PushEmpty(t *testing.T) {
	t.Parallel()
	r := NewRing()
	r.Push("")
	if r.Len() != 0 {
		t.Errorf("Len() after Push(\"\") = %d; want 0", r.Len())
	}
}

// TestRing_PushConsecutiveDedup verifies consecutive duplicates collapse.
// Non-consecutive duplicates do NOT collapse (spec-1.17 R2 D-4).
func TestRing_PushConsecutiveDedup(t *testing.T) {
	t.Parallel()
	r := NewRing()
	r.Push("a")
	r.Push("a")
	r.Push("a")
	if r.Len() != 1 {
		t.Errorf("consecutive dedup Len() = %d; want 1", r.Len())
	}
	r.Push("b")
	r.Push("a") // non-consecutive — should be re-added
	if r.Len() != 3 {
		t.Errorf("non-consecutive dup Len() = %d; want 3", r.Len())
	}
}

// TestRing_Capacity_FIFOEviction fills the Ring past capacity and
// asserts the oldest entries are evicted in insertion order.
func TestRing_Capacity_FIFOEviction(t *testing.T) {
	t.Parallel()
	r := NewRing()
	// Push 300 distinct entries; only the last RingCapacity remain.
	for i := 0; i < 300; i++ {
		r.Push(fmt.Sprintf("entry-%03d", i))
	}
	if r.Len() != RingCapacity {
		t.Errorf("Len() after 300 pushes = %d; want %d", r.Len(), RingCapacity)
	}
	// At(0) should be entry-044 (entries 0..43 evicted; capacity=256).
	want := fmt.Sprintf("entry-%03d", 300-RingCapacity)
	if s, _ := r.At(0); s != want {
		t.Errorf("At(0) after eviction = %q; want %q", s, want)
	}
	// At(Len()-1) should be entry-299 (newest).
	if s, _ := r.At(r.Len() - 1); s != "entry-299" {
		t.Errorf("At(Len()-1) after eviction = %q; want \"entry-299\"", s)
	}
}

// TestRing_AtOutOfRange covers the negative and over-Len cases.
func TestRing_AtOutOfRange(t *testing.T) {
	t.Parallel()
	r := NewRing()
	r.Push("a")
	r.Push("b")
	cases := []int{-1, -100, 2, 999}
	for _, idx := range cases {
		t.Run(fmt.Sprintf("idx=%d", idx), func(t *testing.T) {
			if s, ok := r.At(idx); ok || s != "" {
				t.Errorf("At(%d) = (%q, %v); want (\"\", false)", idx, s, ok)
			}
		})
	}
}

// TestRing_AtAllIndices iterates through a partially-filled ring to
// confirm consecutive At calls return entries in oldest→newest order.
func TestRing_AtAllIndices(t *testing.T) {
	t.Parallel()
	r := NewRing()
	for _, e := range []string{"a", "b", "c", "d"} {
		r.Push(e)
	}
	for i, want := range []string{"a", "b", "c", "d"} {
		if s, ok := r.At(i); !ok || s != want {
			t.Errorf("At(%d) = (%q, %v); want (%q, true)", i, s, ok, want)
		}
	}
}

// BenchmarkInputRing_Push targets spec-1.17 § 4.4 < 200 ns/op including
// dedup check.
func BenchmarkInputRing_Push(b *testing.B) {
	r := NewRing()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Alternate strings so dedup doesn't short-circuit.
		if i%2 == 0 {
			r.Push("alpha")
		} else {
			r.Push("beta")
		}
	}
}
