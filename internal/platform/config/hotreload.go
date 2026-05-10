// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Hot-reload Watcher interface (spec-0.4 D-7, R3 Q5 ★A).
//
// Stage-0 ships NoopWatcher only — Subscribe stores the callback but never
// invokes it. spec-4.6-config-hot-reload replaces NoopWatcher with an
// fsnotify-based real implementation; the interface signatures remain stable
// so downstream consumers (LLM client / sentinel / logger) wire against the
// same contract today.

package config

import "sync"

// Watcher emits new *Config snapshots when the on-disk config changes.
//
// Subscribe registers callback for future config changes; the returned
// unsubscribe function removes the callback (safe to call multiple times).
//
// Close terminates the watcher. After Close, Subscribe is a no-op
// (returns a noop unsubscribe).
type Watcher interface {
	Subscribe(callback func(*Config)) (unsubscribe func())
	Close() error
}

// NoopWatcher implements Watcher with no-op behavior. Used until spec-4.6
// brings fsnotify online.
type NoopWatcher struct {
	mu     sync.Mutex
	closed bool
}

// Subscribe is a no-op; the callback is never invoked.
func (w *NoopWatcher) Subscribe(_ func(*Config)) (unsubscribe func()) {
	return func() {}
}

// Close marks the watcher closed. Idempotent.
func (w *NoopWatcher) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
	return nil
}

// globalWatcher is the package-level Watcher used by Config.Watcher().
// spec-0.4 ships a single shared NoopWatcher; spec-4.6 replaces this with
// an fsnotify-backed instance via SetGlobalWatcher().
var globalWatcher Watcher = &NoopWatcher{}

// SetGlobalWatcher swaps the package-level watcher (used by spec-4.6 and
// tests). Returns the previous watcher so callers can restore.
func SetGlobalWatcher(w Watcher) Watcher {
	prev := globalWatcher
	globalWatcher = w
	return prev
}
