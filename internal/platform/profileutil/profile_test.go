// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package profileutil

import (
	"bytes"
	"strings"
	"sync"
	"testing"
)

func TestCheckpoint_RecordsName(t *testing.T) {
	Reset()
	Checkpoint("a")
	Checkpoint("b")
	Checkpoint("c")
	var buf bytes.Buffer
	Report(&buf)
	out := buf.String()
	for _, name := range []string{"a", "b", "c", "N=3"} {
		if !strings.Contains(out, name) {
			t.Errorf("Report output missing %q.\n--- got ---\n%s", name, out)
		}
	}
}

func TestReport_EmptyIsSilent(t *testing.T) {
	Reset()
	var buf bytes.Buffer
	Report(&buf)
	if buf.Len() != 0 {
		t.Errorf("Report on empty should be silent; got %q", buf.String())
	}
}

func TestCheckpoint_ConcurrentSafe(t *testing.T) {
	Reset()
	const goroutines = 16
	const each = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < each; j++ {
				Checkpoint("concurrent")
			}
		}()
	}
	wg.Wait()
	mu.Lock()
	got := len(entries)
	mu.Unlock()
	if got != goroutines*each {
		t.Errorf("expected %d entries, got %d", goroutines*each, got)
	}
}

func TestReset_Clears(t *testing.T) {
	Reset()
	Checkpoint("first")
	Reset()
	mu.Lock()
	got := len(entries)
	mu.Unlock()
	if got != 0 {
		t.Errorf("after Reset, expected 0 entries, got %d", got)
	}
	// Subsequent Checkpoint should still work and re-init startOnce.
	Checkpoint("after-reset")
	var buf bytes.Buffer
	Report(&buf)
	if !strings.Contains(buf.String(), "after-reset") {
		t.Error("Checkpoint after Reset failed to record")
	}
}

func TestReport_OrderIsDeterministic(t *testing.T) {
	Reset()
	for _, name := range []string{"alpha", "beta", "gamma"} {
		Checkpoint(name)
	}
	var buf bytes.Buffer
	Report(&buf)
	out := buf.String()
	idxA := strings.Index(out, "alpha")
	idxB := strings.Index(out, "beta")
	idxG := strings.Index(out, "gamma")
	if idxA >= idxB || idxB >= idxG {
		t.Errorf("expected order alpha < beta < gamma in output:\n%s", out)
	}
}

func TestCheckpoint_DurationMonotonic(t *testing.T) {
	Reset()
	Checkpoint("0")
	Checkpoint("1")
	mu.Lock()
	defer mu.Unlock()
	if !entries[1].at.After(entries[0].at) || entries[1].at.Equal(entries[0].at) {
		// Allow equal under low-resolution clocks; tolerate "not before"
		if entries[1].at.Before(entries[0].at) {
			t.Errorf("checkpoint timestamps not monotonic: %v before %v", entries[1].at, entries[0].at)
		}
	}
}
