// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Chaos scenarios for the logger (spec § 9 chaos layer).
//
// These tests deliberately exercise failure paths: disk full, panic mid-write,
// signal during BufferedWriter overflow, concurrent session id uniqueness.
// They live in the unit-test package (not tests/chaos/) so the spec-0.5 gate
// catches regressions; long-running scenarios will move to tests/chaos/ when
// spec-0.11/0.11.5 stand up the chaos pipeline.

package logger

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestChaosSidecarDiskFullMainPathContinues — main path keeps writing even
// when the sidecar writeFunc fails (codex MED-3 + spec § 3 guarantee).
func TestChaosSidecarDiskFullMainPathContinues(t *testing.T) {
	resetForTesting(t)
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	mainPath := filepath.Join(tmp, "main.log")
	setArgvForTesting(t, "opendbx", "--debug", "--debug-file", mainPath)

	if err := Init(InitInput{SessionID: "disk-full-chaos"}); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Replace the sidecar writer's writeFn with one that always errors —
	// simulates ENOSPC. We have to reach into the impl to swap, mirroring
	// what a fault injector would do. This is a chaos test, not normal use.
	impl := current.Load()
	if impl.sidecarWriter == nil {
		t.Fatal("sidecar not initialised")
	}
	impl.sidecarWriter.cfg.writeFn = func(string) error {
		return errors.New("ENOSPC: no space left on device")
	}

	for i := 0; i < 10; i++ {
		L().Info("ping", Attr{Key: "event", Value: "ping"})
	}
	if err := Close(); err != nil {
		// Close returns errors.Join(mainErr, sideErr); sideErr is nil because
		// our sidecarWriteFunc returns nil even on internal failure (best-
		// effort contract). main path SHOULD have written successfully.
		t.Logf("Close: %v (acceptable if only sidecar leg failed)", err)
	}

	// Main log MUST exist with all 10 events.
	raw, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("main log missing despite sidecar failure: %v", err)
	}
	lines := strings.Count(string(raw), "ping\n")
	if lines != 10 {
		t.Errorf("main log = %d ping lines, want 10 (sidecar failure leaked to main)", lines)
	}
}

// TestChaosPanicMidWriteFlushesBuffered — when a panic happens after the
// logger has buffered events, GuardPanic must flush them before re-panicking
// (Q13 ★D contract + R-1 mitigation).
func TestChaosPanicMidWriteFlushesBuffered(t *testing.T) {
	resetForTesting(t)
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	mainPath := filepath.Join(tmp, "main.log")
	setArgvForTesting(t, "opendbx", "--debug", "--debug-file", mainPath)

	if err := Init(InitInput{SessionID: "panic-mid-chaos"}); err != nil {
		t.Fatalf("Init: %v", err)
	}

	caught := func() (v any) {
		defer func() { v = recover() }()
		GuardPanic(func() {
			for i := 0; i < 5; i++ {
				L().Info("pre-panic", Attr{Key: "event", Value: "pre"})
			}
			panic("mid-write")
		})
		return nil
	}()
	if caught == nil {
		t.Fatal("GuardPanic swallowed re-panic")
	}

	// Buffered events + process.panic event must be on disk.
	raw, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("main log missing after panic: %v", err)
	}
	got := string(raw)
	if strings.Count(got, "pre-panic") != 5 {
		t.Errorf("main log = %d pre-panic lines, want 5:\n%s", strings.Count(got, "pre-panic"), got)
	}
	if !strings.Contains(got, "process.panic") {
		t.Errorf("main log missing process.panic event:\n%s", got)
	}
}

// TestChaosSignalDuringOverflow — verifies that detached pendingOverflow
// content is not lost when the writer is disposed while an overflow drain
// goroutine is in-flight. The BufferedWriter.Dispose contract Wait()s for
// the drain.
func TestChaosSignalDuringOverflow(t *testing.T) {
	t.Parallel()
	r := &recorder{}
	slow := &slowRecorder{recorder: r, delay: 5 * time.Millisecond}
	cfg := defaultBufferedWriterConfig(slow.writeFn)
	cfg.maxBufferSize = 2 // force overflow rapidly
	bw := newBufferedWriter(cfg)

	const writes = 100
	for i := 0; i < writes; i++ {
		if err := bw.Write("x"); err != nil {
			t.Fatalf("Write err = %v", err)
		}
	}
	// Dispose immediately while goroutines are still draining.
	if err := bw.Dispose(); err != nil {
		t.Fatalf("Dispose: %v", err)
	}

	if got := len(r.joined()); got != writes {
		t.Errorf("after dispose, got %d bytes, want %d (overflow goroutines lost data)", got, writes)
	}
}

// TestChaosConcurrentSessionIDsUnique — R-12 mitigation. Multiple opendbx
// processes generating session ids concurrently must not collide. We
// simulate by spinning many goroutines that each call generateSessionID.
func TestChaosConcurrentSessionIDsUnique(t *testing.T) {
	t.Parallel()
	const goroutines = 64
	const per = 500

	type result struct {
		ids []string
	}
	out := make(chan result, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			ids := make([]string, per)
			for j := 0; j < per; j++ {
				ids[j] = generateSessionID()
			}
			out <- result{ids: ids}
		}()
	}
	wg.Wait()
	close(out)

	seen := make(map[string]struct{}, goroutines*per)
	for r := range out {
		for _, id := range r.ids {
			if _, dup := seen[id]; dup {
				t.Fatalf("duplicate session id under concurrent generation: %s", id)
			}
			seen[id] = struct{}{}
		}
	}
}

// TestChaosWithContextAcrossGoroutines — span carried in ctx propagates
// correctly into a goroutine that does a logger.Info. We're verifying that
// nothing in WithContext / log() races under the race detector.
func TestChaosWithContextAcrossGoroutines(t *testing.T) {
	resetForTesting(t)
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	mainPath := filepath.Join(tmp, "main.log")
	setArgvForTesting(t, "opendbx", "--debug", "--debug-file", mainPath)

	if err := Init(InitInput{SessionID: "ctx-race"}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	ctx, sp := StartSpan(context.Background(), "tool.parallel")

	const fanout = 32
	var wg sync.WaitGroup
	var total atomic.Int32
	wg.Add(fanout)
	for i := 0; i < fanout; i++ {
		go func() {
			defer wg.Done()
			L().WithContext(ctx).Info("step", Attr{Key: "event", Value: "step"})
			total.Add(1)
		}()
	}
	wg.Wait()
	sp.End()
	if err := Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if total.Load() != fanout {
		t.Errorf("only %d/%d goroutines reported", total.Load(), fanout)
	}

	sidecar := filepath.Join(tmp, ".opendbx", "debug", "ctx-race.events.jsonl")
	raw, err := os.ReadFile(sidecar)
	if err != nil {
		t.Fatalf("sidecar: %v", err)
	}
	// Every step event must carry the same trace_id (the active span's).
	spImpl := sp.(*span)
	hits := strings.Count(string(raw), `"trace_id":"`+spImpl.traceID+`"`)
	// fanout step lines + 1 span.end line, all sharing trace_id.
	if hits < fanout+1 {
		t.Errorf("got %d sidecar lines with trace_id %q, want ≥ %d", hits, spImpl.traceID, fanout+1)
	}
}
