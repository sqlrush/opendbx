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
//   - 4-field cacheKey: sha256(source)+cols+verbose+themeKey (R3 added
//     themeKey; rendered-buffer cache must vary by concrete theme).
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
	// markdownCacheCap is the maximum number of entries the LRU holds
	// before evicting the least-recently-used entry. ~1MB typical
	// memory at 256 entries × ~4KB average rendered buffer.
	markdownCacheCap = 256

	// markdownCacheMaxSourceBytes is the soft cap on cacheable source
	// size (Q7 ★B). Sources larger than this render normally but skip
	// cache lookup/put to bound pathological memory usage.
	markdownCacheMaxSourceBytes = 256 * 1024
)

// lruEntry is a doubly-linked-list element storing a cached rendered
// buffer keyed by cacheKey.
type lruEntry struct {
	key string
	buf buffer.Buffer
}

// markdownLRU implements a simple LRU cache: map + doubly-linked list.
// Guarded by sync.Mutex for concurrent Render calls.
type markdownLRU struct {
	mu    sync.Mutex
	cap   int
	items map[string]*list.Element
	order *list.List // front = most recent
}

// newLRU constructs an LRU with the given capacity.
func newLRU(cap int) *markdownLRU {
	return &markdownLRU{
		cap:   cap,
		items: make(map[string]*list.Element, cap),
		order: list.New(),
	}
}

// Get returns the cached buffer for the key, or nil if absent. Hits
// promote the entry to the front of the LRU list.
func (l *markdownLRU) Get(key string) buffer.Buffer {
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
func (l *markdownLRU) Put(key string, buf buffer.Buffer) {
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
func (l *markdownLRU) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.order.Len()
}

// markdownCache is the package-level singleton (Q5 ★A; R2 MED-1
// initialized via package-level var, no init() side-effect).
var markdownCache = newLRU(markdownCacheCap)

// resetMarkdownCacheForTest clears the singleton cache. Test-only;
// invoked by *_test.go to avoid cross-test pollution. NOT exported
// to non-test callers — guarded by package-private name.
func resetMarkdownCacheForTest() {
	markdownCache.mu.Lock()
	defer markdownCache.mu.Unlock()
	markdownCache.items = make(map[string]*list.Element, markdownCache.cap)
	markdownCache.order = list.New()
}

// makeCacheKey computes the 5-field cacheKey per spec § 3.3 (R3 + R7 HIGH-1).
// 16-byte sha256 prefix gives 2^-64 collision risk at 256-entry cap.
//
// R7 HIGH-1: wrap is included because the walker invokes wrap() with
// ctx.Wrap policy when building physical rows; same source + cols +
// verbose + theme but WrapSoft vs WrapNone produces different output.
func makeCacheKey(source string, cols int, verbose bool, themeKey string, wrap WrapPolicy) string {
	sum := sha256.Sum256([]byte(source))
	return fmt.Sprintf("%x:%d:%t:%s:%d", sum[:16], cols, verbose, themeKey, wrap)
}

// themeCacheKey returns a stable cache-key string for the given theme.
// 3-step fallback ladder per spec § 3.3 R3.1 contract:
//
//  1. nil → "default" sentinel (treated as DefaultTheme).
//  2. theme implements MarkdownCacheKey() → use its return.
//  3. fallback → fmt.Sprintf("%T", theme) (type name only).
//
// Implementations must NOT use %p (unstable across instances) or
// reflect.DeepEqual (O(n) per Render). Stage 1 DefaultTheme is
// stateless so Step 3 suffices.
func themeCacheKey(theme StyleTheme) string {
	if theme == nil {
		return "default"
	}
	if k, ok := theme.(markdownKeyer); ok {
		return k.MarkdownCacheKey()
	}
	return fmt.Sprintf("%T", theme)
}
