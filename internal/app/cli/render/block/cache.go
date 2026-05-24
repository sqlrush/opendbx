// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File cache.go — spec-1.11 D-4 LRU buffer cache for Markdown block.
//
// Design (R3 + R3.1):
//   - Package-level singleton (Q5 ★A): one cache shared across all
//     Markdown instances. Maximizes hit rate when scrollback re-renders
//     historical messages.
//   - 256-entry cap (user-locked R2.1; ~1MB typical / 64MB pathological
//     bound with 256KB source no-cache guard).
//   - 5-field cacheKey: sha256(source)+cols+verbose+themeKey+wrap (R3
//     added themeKey for theme variance; R7 HIGH-1 added wrap because
//     WrapSoft/Hard/None produce different cell grids).
//   - I-8 immutability contract: cache returns Buffer by reference;
//     caller must treat as read-only. Cache hit < 5µs (map lookup only).
//   - sync.Mutex guards map + LRU list only (NOT parse). Single-flight
//     not implemented per I-2 (KISS; worst-case 2x re-parse acceptable
//     for ≤ 100 RPS workload).
//   - themeCacheKey 3-step fallback (R3.1 contract lock).

package block

import (
	"container/list"
	"crypto/sha256"
	"fmt"
	"sync"

	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
)

const (
	// blockCacheCap is the maximum number of entries the LRU holds
	// before evicting the least-recently-used entry. ~1MB typical
	// memory at 256 entries × ~4KB average rendered buffer.
	blockCacheCap = 256

	// blockCacheMaxSourceBytes is the soft cap on cacheable source
	// size (Q7 ★B). Sources larger than this render normally but skip
	// cache lookup/put to bound pathological memory usage.
	blockCacheMaxSourceBytes = 256 * 1024
)

// lruEntry is a doubly-linked-list element storing a cached rendered
// buffer keyed by cacheKey.
type lruEntry struct {
	key string
	buf buffer.Buffer
}

// blockLRU implements a simple LRU cache: map + doubly-linked list.
// Guarded by sync.Mutex for concurrent Render calls.
type blockLRU struct {
	mu    sync.Mutex
	cap   int
	items map[string]*list.Element
	order *list.List // front = most recent
}

// newLRU constructs an LRU with the given capacity.
func newLRU(cap int) *blockLRU {
	return &blockLRU{
		cap:   cap,
		items: make(map[string]*list.Element, cap),
		order: list.New(),
	}
}

// Get returns the cached buffer for the key, or nil if absent. Hits
// promote the entry to the front of the LRU list.
func (l *blockLRU) Get(key string) buffer.Buffer {
	l.mu.Lock()
	defer l.mu.Unlock()
	if el, ok := l.items[key]; ok {
		l.order.MoveToFront(el)
		return el.Value.(*lruEntry).buf
	}
	return nil
}

// Put stores the buffer under the key. Existing entries are updated
// in place. New entries that exceed capacity evict the back of the
// LRU list.
func (l *blockLRU) Put(key string, buf buffer.Buffer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if el, ok := l.items[key]; ok {
		el.Value.(*lruEntry).buf = buf
		l.order.MoveToFront(el)
		return
	}
	entry := &lruEntry{key: key, buf: buf}
	el := l.order.PushFront(entry)
	l.items[key] = el
	if l.order.Len() > l.cap {
		evict := l.order.Back()
		if evict != nil {
			l.order.Remove(evict)
			delete(l.items, evict.Value.(*lruEntry).key)
		}
	}
}

// Len returns the current number of cached entries.
func (l *blockLRU) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.order.Len()
}

// blockCache is the package-level singleton (Q5 ★A; R2 MED-1
// initialized via package-level var, no init() side-effect).
var blockCache = newLRU(blockCacheCap)

// resetBlockCacheForTest clears the singleton cache. Test-only;
// invoked by *_test.go to avoid cross-test pollution. NOT exported
// to non-test callers — guarded by package-private name.
func resetBlockCacheForTest() {
	blockCache.mu.Lock()
	defer blockCache.mu.Unlock()
	blockCache.items = make(map[string]*list.Element, blockCache.cap)
	blockCache.order = list.New()
}

// makeBlockCacheKey computes the 8-field cacheKey per spec § 3.3 (R3 +
// R7 HIGH-1 + spec-1.12 D-7 + R2 HIGH-3 collision math fix).
// 16-byte sha256 prefix gives ~2^-112 collision risk at 256-entry cap
// (128-bit hash, n²/2^bits ≈ 256²/2^128 ≈ 2^-112; previously documented
// as 2^-64 in spec-1.11 — off by factor 2^48).
//
// R7 HIGH-1: wrap is included because the walker invokes wrap() with
// ctx.Wrap policy when building physical rows.
// spec-1.12 D-7: lang + codeStyleName + colorDepth added because chroma
// highlight output varies per-language, per-style, and per-color-depth.
func makeBlockCacheKey(source string, cols int, verbose bool, themeKey string, wrap WrapPolicy, lang, codeStyleName string, colorDepth int) string {
	sum := sha256.Sum256([]byte(source))
	return fmt.Sprintf("%x:%d:%t:%s:%d:%s:%s:%d", sum[:16], cols, verbose, themeKey, wrap, lang, codeStyleName, colorDepth)
}

// themeCacheKey returns a stable cache-key string for the given theme.
// 3-step fallback ladder per spec § 3.3 R3.1 contract:
//
//  1. nil → "default" sentinel (treated as DefaultTheme).
//  2. theme implements BlockCacheKey() → use its return.
//  3. fallback → fmt.Sprintf("%T", theme) (type name only).
//
// Implementations must NOT use %p (unstable across instances) or
// reflect.DeepEqual (O(n) per Render). Stage 1 DefaultTheme is
// stateless so Step 3 suffices.
func themeCacheKey(theme StyleTheme) string {
	if theme == nil {
		return "default"
	}
	if k, ok := theme.(blockKeyer); ok {
		return k.BlockCacheKey()
	}
	return fmt.Sprintf("%T", theme)
}
