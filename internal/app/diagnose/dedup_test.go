// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package diagnose

import (
	"testing"

	"github.com/sqlrush/opendbx/internal/domain/llm"
)

func mustDecode(t *testing.T, raw string) map[string]any {
	t.Helper()
	m, err := llm.DecodeToolInput([]byte(raw))
	if err != nil {
		t.Fatalf("DecodeToolInput(%q): %v", raw, err)
	}
	return m
}

func keyOf(t *testing.T, name string, input map[string]any) string {
	t.Helper()
	k, err := dedupKey(name, input)
	if err != nil {
		t.Fatalf("dedupKey(%q): %v", name, err)
	}
	return k
}

// TestDedupKey_ConstructionOrderIndependent — json.Marshal sorts map keys, so
// the literal construction order of a map must not change the hash.
func TestDedupKey_ConstructionOrderIndependent(t *testing.T) {
	t.Parallel()
	a := map[string]any{"x": "1", "y": "2"}
	b := map[string]any{"y": "2", "x": "1"}
	if keyOf(t, "echo", a) != keyOf(t, "echo", b) {
		t.Fatal("same-content maps must produce the same key (json.Marshal key-sort)")
	}
}

func TestDedupKey_DifferentInput_DifferentKey(t *testing.T) {
	t.Parallel()
	if keyOf(t, "echo", map[string]any{"x": "1"}) == keyOf(t, "echo", map[string]any{"x": "2"}) {
		t.Fatal("different Input must produce different key")
	}
}

// TestDedupKey_NameIsolation — same Input under different tool names must not
// collide (name prefix isolates the key space).
func TestDedupKey_NameIsolation(t *testing.T) {
	t.Parallel()
	in := map[string]any{"x": "1"}
	if keyOf(t, "echo", in) == keyOf(t, "clock", in) {
		t.Fatal("same Input under different name must produce different key")
	}
}

// TestDedupKey_DecodeToolInput_Deterministic — the real Input path is
// llm.DecodeToolInput (UseNumber). Same wire content, different field order →
// identical key (the json.Number canonical-determinism invariant, spec-1.22
// §1.1 C / R-3).
func TestDedupKey_DecodeToolInput_Deterministic(t *testing.T) {
	t.Parallel()
	k1 := keyOf(t, "echo", mustDecode(t, `{"n":100,"s":"hi"}`))
	k2 := keyOf(t, "echo", mustDecode(t, `{"s":"hi","n":100}`))
	if k1 != k2 {
		t.Fatalf("DecodeToolInput same-content different-order must hash equal:\n%s\n%s", k1, k2)
	}
}

// TestDedupKey_JSONNumber_IntVsFloat_DifferentKey_KnownLimitation pins the
// documented known limitation: `1` and `1.0` are distinct json.Number
// representations and are intentionally NOT semantically normalized, so the
// same logical value with different wire representation is a cache MISS
// (spec-1.22 §1.1 C / R-3 — behavior, not a bug). If this assertion ever
// flips, the spec must be updated deliberately.
func TestDedupKey_JSONNumber_IntVsFloat_DifferentKey_KnownLimitation(t *testing.T) {
	t.Parallel()
	one := keyOf(t, "echo", mustDecode(t, `{"n":1}`))
	onePointZero := keyOf(t, "echo", mustDecode(t, `{"n":1.0}`))
	if one == onePointZero {
		t.Fatal("known-limitation invariant changed: 1 and 1.0 now hash equal (update spec-1.22 R-3 if intentional)")
	}
}

// TestDedupCache_WindowHit — a stored entry is reused for turns within the
// window: stored at turn 1, window 3 → hits at turns 1, 2, 3 (diff 0,1,2 < 3).
func TestDedupCache_WindowHit(t *testing.T) {
	t.Parallel()
	c := newDedupCache(true, 3)
	c.store("k", ToolOutput{Content: "result"}, 1)
	for _, turn := range []int{1, 2, 3} {
		got, ok := c.lookup("k", turn)
		if !ok || got.Content != "result" {
			t.Fatalf("turn %d: want hit result, got ok=%v %+v", turn, ok, got)
		}
	}
}

// TestDedupCache_WindowExpiry — boundary: an entry stored at turn T expires at
// turn T+window (the first non-hit). diff == window → miss.
func TestDedupCache_WindowExpiry(t *testing.T) {
	t.Parallel()
	c := newDedupCache(true, 3)
	c.store("k", ToolOutput{Content: "r"}, 1)
	if _, ok := c.lookup("k", 4); ok { // 4-1 == 3 == window → expired
		t.Fatal("entry must expire at storedTurn+window (boundary)")
	}
}

func TestDedupCache_Disabled_AllMiss(t *testing.T) {
	t.Parallel()
	c := newDedupCache(false, 3)
	c.store("k", ToolOutput{Content: "r"}, 1)
	if _, ok := c.lookup("k", 1); ok {
		t.Fatal("disabled cache must never hit")
	}
}

// TestDedupCache_StoreSkipsError — defense-in-depth: store ignores IsError
// results so a caller bug cannot cache an error as a future success (the Loop
// additionally guards execErr==nil, which the cache cannot observe).
func TestDedupCache_StoreSkipsError(t *testing.T) {
	t.Parallel()
	c := newDedupCache(true, 3)
	c.store("k", ToolOutput{Content: "boom", IsError: true}, 1)
	if _, ok := c.lookup("k", 1); ok {
		t.Fatal("IsError result must not be cached")
	}
}

func TestDedupCache_NilSafe(t *testing.T) {
	t.Parallel()
	var c *dedupCache
	if _, ok := c.lookup("k", 1); ok {
		t.Fatal("nil cache lookup must miss")
	}
	c.store("k", ToolOutput{}, 1) // must not panic
}
