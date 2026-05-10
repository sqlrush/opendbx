// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package config

import "testing"

func TestSourceTracking(t *testing.T) {
	cfg := Default()
	if got := cfg.Source("Security"); got != SourceDefault {
		t.Errorf("fresh Default Source(Security) = %v, want SourceDefault", got)
	}
	cfg.SetSource("Security", SourceUserSettings)
	if got := cfg.Source("Security"); got != SourceUserSettings {
		t.Errorf("after SetSource: got %v, want SourceUserSettings", got)
	}
	if got := cfg.Source("UnknownField"); got != SourceDefault {
		t.Errorf("Source on unknown field returned %v, want SourceDefault", got)
	}
}

func TestWatcherIsNoop(t *testing.T) {
	cfg := Default()
	w := cfg.Watcher()
	if w == nil {
		t.Fatal("Watcher returned nil")
	}
	called := false
	unsub := w.Subscribe(func(*Config) { called = true })
	if unsub == nil {
		t.Fatal("Subscribe returned nil unsubscribe")
	}
	unsub()
	if called {
		t.Error("NoopWatcher invoked callback (should be no-op)")
	}
	if err := w.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestSetGlobalWatcher_Restore(t *testing.T) {
	original := globalWatcher
	defer func() { globalWatcher = original }()

	type fakeWatcher struct{ NoopWatcher }
	new := &fakeWatcher{}
	prev := SetGlobalWatcher(new)
	if prev != original {
		t.Error("SetGlobalWatcher did not return the previous watcher")
	}
	if globalWatcher != new {
		t.Error("globalWatcher not swapped")
	}
}
