// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package logger

import (
	"regexp"
	"sync"
	"testing"
)

// canonicalUUIDRE matches the RFC 4122 8-4-4-4-12 hex form. Version &
// variant constraints checked separately via the dedicated assertions.
var canonicalUUIDRE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func TestUUID4Format(t *testing.T) {
	t.Parallel()
	got := uuid4()
	if !canonicalUUIDRE.MatchString(got) {
		t.Fatalf("uuid4() = %q, want canonical 8-4-4-4-12 hex", got)
	}
	// Version nibble at index 14 (after the second hyphen).
	if got[14] != '4' {
		t.Errorf("version nibble = %q, want '4'", got[14])
	}
	// Variant high bits at index 19 must be 8, 9, a, or b (10xx binary).
	switch got[19] {
	case '8', '9', 'a', 'b':
	default:
		t.Errorf("variant nibble = %q, want 8/9/a/b", got[19])
	}
}

func TestUUID4Uniqueness(t *testing.T) {
	t.Parallel()
	const n = 10000
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		u := uuid4()
		if _, dup := seen[u]; dup {
			t.Fatalf("duplicate uuid4 at i=%d: %s", i, u)
		}
		seen[u] = struct{}{}
	}
}

func TestUUID7Format(t *testing.T) {
	t.Parallel()
	got := uuid7()
	if !canonicalUUIDRE.MatchString(got) {
		t.Fatalf("uuid7() = %q, want canonical hex form", got)
	}
	if got[14] != '7' {
		t.Errorf("version nibble = %q, want '7'", got[14])
	}
	switch got[19] {
	case '8', '9', 'a', 'b':
	default:
		t.Errorf("variant nibble = %q, want 8/9/a/b", got[19])
	}
}

// TestUUID7TimeOrdering — sequential calls must produce lexicographically
// non-decreasing UUIDs. The 48-bit ms prefix + monotonic counter guarantees
// this even when many UUIDs are generated within a single ms.
func TestUUID7TimeOrdering(t *testing.T) {
	t.Parallel()
	prev := uuid7()
	for i := 0; i < 5000; i++ {
		next := uuid7()
		if next <= prev {
			t.Fatalf("uuid7 not monotonic at i=%d: prev=%s next=%s", i, prev, next)
		}
		prev = next
	}
}

// TestUUID7Concurrent — concurrent uuid7() calls must remain unique under
// race detector. Monotonic counter prevents intra-ms collisions.
func TestUUID7Concurrent(t *testing.T) {
	t.Parallel()
	const goroutines = 16
	const per = 1000
	var mu sync.Mutex
	seen := make(map[string]struct{}, goroutines*per)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < per; j++ {
				u := uuid7()
				mu.Lock()
				if _, dup := seen[u]; dup {
					mu.Unlock()
					t.Errorf("duplicate uuid7: %s", u)
					return
				}
				seen[u] = struct{}{}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
}
