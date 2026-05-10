// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package config

import (
	"sync"
	"testing"
)

func TestNoopWatcher_SubscribeReturnsUnsubscribe(t *testing.T) {
	w := &NoopWatcher{}
	unsub := w.Subscribe(func(*Config) {})
	if unsub == nil {
		t.Fatal("Subscribe returned nil unsubscribe")
	}
	unsub() // should not panic
	unsub() // idempotent
}

func TestNoopWatcher_NeverInvokesCallback(t *testing.T) {
	w := &NoopWatcher{}
	called := 0
	w.Subscribe(func(*Config) { called++ })
	if called != 0 {
		t.Errorf("NoopWatcher invoked callback %d times (should be 0)", called)
	}
}

func TestNoopWatcher_CloseIdempotent(t *testing.T) {
	w := &NoopWatcher{}
	if err := w.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

func TestNoopWatcher_NilCallback(t *testing.T) {
	w := &NoopWatcher{}
	// nil callback should still produce a working unsubscribe.
	unsub := w.Subscribe(nil)
	if unsub == nil {
		t.Fatal("Subscribe(nil) returned nil unsubscribe")
	}
	unsub()
}

func TestNoopWatcher_ConcurrentSubscribe(t *testing.T) {
	w := &NoopWatcher{}
	const n = 32
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			u := w.Subscribe(func(*Config) {})
			u()
		}()
	}
	wg.Wait()
}

func TestSetGlobalWatcher_Returns(t *testing.T) {
	original := globalWatcher
	defer func() { globalWatcher = original }()

	type alt struct{ NoopWatcher }
	a := &alt{}
	prev := SetGlobalWatcher(a)
	if prev != original {
		t.Errorf("SetGlobalWatcher returned %v, want %v", prev, original)
	}
	// Restore via second swap.
	SetGlobalWatcher(prev)
}

func TestConfigWatcher_ReturnsGlobal(t *testing.T) {
	cfg := Default()
	if cfg.Watcher() == nil {
		t.Fatal("Config.Watcher() returned nil")
	}
}
