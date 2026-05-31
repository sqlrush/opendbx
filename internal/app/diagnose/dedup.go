// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File dedup.go — per-Run tool-call dedup cache (spec-1.22 D-1).
//
// Within a single Loop.Run, a tool call whose (Name, canonical-hash(Input))
// matches a recent successful call returns the cached ToolOutput instead of
// re-running Execute. This defends 痛点 1.7 / re-frames master-test-catalog
// E-3 (the model wastes turns re-issuing identical queries in long chains).
//
// The "cached" signal is surfaced to the UI as a render-only flag
// (block.ToolResult.Cached, spec-1.22 D-6); per CLAUDE.md § 3.6 errata the
// provider/wire/transcript content stays byte-identical to a fresh run — no
// marker is injected (invariant #2).
//
// Key: tool name + sha256(json.Marshal(Input)). json.Marshal sorts map keys
// (Go stdlib), so a map's literal construction order does not affect the hash.
// Input always arrives via llm.DecodeToolInput (UseNumber), so numeric values
// are canonical json.Number strings that hash deterministically across turns —
// but `1` and `1.0` are distinct representations and are intentionally NOT
// normalized (a known limitation, spec-1.22 §1.1 C / R-3; a cache miss for
// same-semantic input, not a bug).
//
// Lifetime: one dedupCache per Loop.Run (in-memory; cross-Run/session caching
// is spec-2.12). NOT goroutine-safe — Loop calls it serially from Phase 2
// (spec-1.21 D-4 串行约定).

package diagnose

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// CacheableTool is an OPTIONAL interface. A ToolExecutor that implements it
// and returns false opts out of dedup caching — for time-sensitive or
// non-idempotent tools (e.g. clock, whose value changes every call). Tools
// that do NOT implement it default to cacheable (spec-1.22 D-4).
type CacheableTool interface {
	Cacheable() bool
}

// dedupKey derives the cache key for a tool call. Callers MUST first confirm
// the tool is cacheable and dedup is enabled (spec-1.22 invariant #5: a
// non-cacheable / disabled call derives no key).
func dedupKey(name string, input map[string]any) (string, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return name + ":" + hex.EncodeToString(sum[:]), nil
}

// dedupEntry is a cached successful result tagged with the turn it was stored.
type dedupEntry struct {
	out  ToolOutput
	turn int
}

// dedupCache is per-Run state. window is the turn-distance within which a
// stored entry is reused; enabled=false makes every lookup a miss and every
// store a no-op (the disable path — spec-1.22 D-5 tri-state).
type dedupCache struct {
	enabled bool
	window  int
	entries map[string]dedupEntry
}

// newDedupCache builds an (empty) per-Run cache.
func newDedupCache(enabled bool, window int) *dedupCache {
	return &dedupCache{enabled: enabled, window: window, entries: map[string]dedupEntry{}}
}

// lookup returns (out, true) iff dedup is enabled, an entry exists for key, and
// it was stored within `window` turns (curTurn - storedTurn < window). An
// expired entry is treated as a miss; a later store overwrites it.
func (c *dedupCache) lookup(key string, curTurn int) (ToolOutput, bool) {
	if c == nil || !c.enabled {
		return ToolOutput{}, false
	}
	e, ok := c.entries[key]
	if !ok || curTurn-e.turn >= c.window {
		return ToolOutput{}, false
	}
	return e.out, true
}

// store records a SUCCESSFUL result. The Loop guards execErr==nil before
// calling (the cache cannot observe a Go error return); store additionally
// skips IsError results as defense-in-depth so an error can never be cached as
// a future success (spec-1.22 R-4). No-op when the cache is disabled.
func (c *dedupCache) store(key string, out ToolOutput, curTurn int) {
	if c == nil || !c.enabled || out.IsError {
		return
	}
	c.entries[key] = dedupEntry{out: out, turn: curTurn}
}
