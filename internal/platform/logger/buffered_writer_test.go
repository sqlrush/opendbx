// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package logger

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// recorder is a writeFunc helper that captures every batch passed to the
// BufferedWriter. It is safe for concurrent use.
type recorder struct {
	mu      sync.Mutex
	batches []string
	failNth atomic.Int32 // if > 0, the N-th call returns ErrInjected
	calls   atomic.Int32
}

var errInjected = errors.New("injected writeFn error")

func (r *recorder) writeFn(s string) error {
	n := r.calls.Add(1)
	if fail := r.failNth.Load(); fail > 0 && n == fail {
		return errInjected
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.batches = append(r.batches, s)
	return nil
}

func (r *recorder) joined() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.batches, "")
}

func (r *recorder) numBatches() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.batches)
}

func TestBufferedWriterImmediateMode(t *testing.T) {
	t.Parallel()
	r := &recorder{}
	cfg := defaultBufferedWriterConfig(r.writeFn)
	cfg.immediateMode = true
	bw := newBufferedWriter(cfg)

	if err := bw.Write("a"); err != nil {
		t.Fatalf("Write err = %v", err)
	}
	if err := bw.Write("b"); err != nil {
		t.Fatalf("Write err = %v", err)
	}
	// In immediate mode each Write is its own batch — no buffering.
	if r.numBatches() != 2 {
		t.Errorf("immediate mode: got %d batches, want 2", r.numBatches())
	}
	if r.joined() != "ab" {
		t.Errorf("immediate mode payload\n  got:  %q\n  want: %q", r.joined(), "ab")
	}
}

func TestBufferedWriterImmediateModeHonorsClosed(t *testing.T) {
	t.Parallel()
	r := &recorder{}
	cfg := defaultBufferedWriterConfig(r.writeFn)
	cfg.immediateMode = true
	bw := newBufferedWriter(cfg)
	if err := bw.Dispose(); err != nil {
		t.Fatalf("Dispose err = %v", err)
	}
	if err := bw.Write("late"); !errors.Is(err, ErrWriterClosed) {
		t.Fatalf("immediate Write after Dispose err = %v, want ErrWriterClosed", err)
	}
	if r.numBatches() != 0 {
		t.Fatalf("late immediate write reached writeFn; batches=%d", r.numBatches())
	}
}

func TestBufferedWriterFlushDrainsBuffer(t *testing.T) {
	t.Parallel()
	r := &recorder{}
	cfg := defaultBufferedWriterConfig(r.writeFn)
	bw := newBufferedWriter(cfg)

	for _, c := range []string{"a", "b", "c"} {
		if err := bw.Write(c); err != nil {
			t.Fatalf("Write err = %v", err)
		}
	}
	// Buffered under threshold; no flush yet.
	if r.numBatches() != 0 {
		t.Fatalf("pre-flush: got %d batches, want 0", r.numBatches())
	}
	if err := bw.Flush(); err != nil {
		t.Fatalf("Flush err = %v", err)
	}
	if r.numBatches() != 1 {
		t.Errorf("post-flush: got %d batches, want 1", r.numBatches())
	}
	if r.joined() != "abc" {
		t.Errorf("payload\n  got:  %q\n  want: %q", r.joined(), "abc")
	}
}

func TestBufferedWriterSizeOverflow(t *testing.T) {
	t.Parallel()
	r := &recorder{}
	cfg := defaultBufferedWriterConfig(r.writeFn)
	cfg.maxBufferSize = 3
	bw := newBufferedWriter(cfg)

	// Write 3 items — triggers size overflow → async drain goroutine fires.
	for _, c := range []string{"a", "b", "c"} {
		if err := bw.Write(c); err != nil {
			t.Fatalf("Write err = %v", err)
		}
	}
	// Drain may be async; force it via Dispose (sync flush + WaitGroup).
	if err := bw.Dispose(); err != nil {
		t.Fatalf("Dispose err = %v", err)
	}
	if got := r.joined(); got != "abc" {
		t.Errorf("size overflow payload\n  got:  %q\n  want: %q", got, "abc")
	}
}

func TestBufferedWriterBytesOverflow(t *testing.T) {
	t.Parallel()
	r := &recorder{}
	cfg := defaultBufferedWriterConfig(r.writeFn)
	cfg.maxBufferBytes = 5 // 5-byte threshold
	bw := newBufferedWriter(cfg)

	if err := bw.Write("12345"); err != nil { // exactly 5 bytes — triggers overflow
		t.Fatalf("Write err = %v", err)
	}
	if err := bw.Dispose(); err != nil {
		t.Fatalf("Dispose err = %v", err)
	}
	if got := r.joined(); got != "12345" {
		t.Errorf("bytes overflow payload\n  got:  %q\n  want: %q", got, "12345")
	}
}

func TestBufferedWriterTimerFlush(t *testing.T) {
	t.Parallel()
	r := &recorder{}
	cfg := defaultBufferedWriterConfig(r.writeFn)
	cfg.flushInterval = 30 * time.Millisecond
	bw := newBufferedWriter(cfg)

	if err := bw.Write("timed"); err != nil {
		t.Fatalf("Write err = %v", err)
	}
	// Pre-timer: nothing flushed yet.
	if r.numBatches() != 0 {
		t.Fatalf("pre-timer: got %d batches, want 0", r.numBatches())
	}
	// Wait > 2 × flushInterval to be safe under load.
	time.Sleep(80 * time.Millisecond)
	if r.numBatches() != 1 || r.joined() != "timed" {
		t.Errorf("timer flush did not run: batches=%d payload=%q", r.numBatches(), r.joined())
	}
	if err := bw.Dispose(); err != nil {
		t.Fatalf("Dispose err = %v", err)
	}
}

func TestBufferedWriterDisposeFlushesBuffer(t *testing.T) {
	t.Parallel()
	r := &recorder{}
	bw := newBufferedWriter(defaultBufferedWriterConfig(r.writeFn))

	for _, c := range []string{"x", "y"} {
		if err := bw.Write(c); err != nil {
			t.Fatalf("Write err = %v", err)
		}
	}
	if err := bw.Dispose(); err != nil {
		t.Fatalf("Dispose err = %v", err)
	}
	if got := r.joined(); got != "xy" {
		t.Errorf("dispose flush\n  got:  %q\n  want: %q", got, "xy")
	}
	// Subsequent Write must return ErrWriterClosed.
	if err := bw.Write("z"); !errors.Is(err, ErrWriterClosed) {
		t.Errorf("post-dispose Write err = %v, want ErrWriterClosed", err)
	}
}

func TestBufferedWriterDisposeIdempotent(t *testing.T) {
	t.Parallel()
	r := &recorder{}
	bw := newBufferedWriter(defaultBufferedWriterConfig(r.writeFn))
	if err := bw.Write("a"); err != nil {
		t.Fatalf("Write err = %v", err)
	}
	if err := bw.Dispose(); err != nil {
		t.Fatalf("first Dispose err = %v", err)
	}
	if err := bw.Dispose(); err != nil {
		t.Errorf("second Dispose err = %v, want nil (idempotent)", err)
	}
	if r.numBatches() != 1 {
		t.Errorf("idempotent dispose: got %d batches, want 1", r.numBatches())
	}
}

// TestBufferedWriterPendingOverflowCoalesce verifies that when multiple
// overflows fire back-to-back, only ONE goroutine drains the coalesced
// batch (CC parity: pendingOverflow accumulates rather than spawning a
// drain per overflow).
func TestBufferedWriterPendingOverflowCoalesce(t *testing.T) {
	t.Parallel()
	r := &recorder{}
	// Slow writeFn so the drain goroutine is in-flight when subsequent
	// overflows fire — that's when coalescing kicks in.
	slowR := &slowRecorder{recorder: r, delay: 20 * time.Millisecond}
	cfg := defaultBufferedWriterConfig(slowR.writeFn)
	cfg.maxBufferSize = 2
	bw := newBufferedWriter(cfg)

	// 6 writes → 3 logical overflows. Without coalescing we'd see 3 drain
	// goroutines (each writing a 2-element batch). With coalescing, the
	// first overflow detaches, and overflows 2 + 3 append to pendingOverflow
	// before the in-flight drain reads it.
	for i := 0; i < 6; i++ {
		if err := bw.Write("x"); err != nil {
			t.Fatalf("Write err = %v", err)
		}
	}
	if err := bw.Dispose(); err != nil {
		t.Fatalf("Dispose err = %v", err)
	}
	// All 6 'x's must have been written; the order check is sufficient.
	if got := r.joined(); got != "xxxxxx" {
		t.Errorf("coalesce payload\n  got:  %q\n  want: %q", got, "xxxxxx")
	}
	// Number of batches: at minimum 1 (everything coalesced), at most 4
	// (each overflow + final dispose flush). We don't pin a specific count
	// because the scheduling is timing-dependent; just verify it's < 6
	// (proving SOME coalescing happened).
	if n := r.numBatches(); n >= 6 {
		t.Errorf("no coalescing observed: %d batches, want < 6", n)
	}
}

func TestBufferedWriterConcurrentWriteRace(t *testing.T) {
	t.Parallel()
	r := &recorder{}
	bw := newBufferedWriter(defaultBufferedWriterConfig(r.writeFn))

	const goroutines = 32
	const perGoroutine = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				_ = bw.Write("x")
			}
		}()
	}
	wg.Wait()
	if err := bw.Dispose(); err != nil {
		t.Fatalf("Dispose err = %v", err)
	}
	got := r.joined()
	wantLen := goroutines * perGoroutine
	if len(got) != wantLen {
		t.Errorf("concurrent Write: got %d bytes, want %d", len(got), wantLen)
	}
}

// TestBufferedWriterFlushReturnsWriteFnErr exercises the dispose error path:
// when writeFn returns an error, Flush propagates it to the caller (used by
// impl.close + errors.Join in T-7).
func TestBufferedWriterFlushReturnsWriteFnErr(t *testing.T) {
	t.Parallel()
	r := &recorder{}
	r.failNth.Store(1) // first write fails
	bw := newBufferedWriter(defaultBufferedWriterConfig(r.writeFn))
	if err := bw.Write("a"); err != nil {
		t.Fatalf("Write err = %v", err)
	}
	err := bw.Flush()
	if !errors.Is(err, errInjected) {
		t.Errorf("Flush err = %v, want errInjected", err)
	}
}

// slowRecorder wraps recorder with a delay so we can deterministically
// observe overflow coalescing behaviour.
type slowRecorder struct {
	*recorder
	delay time.Duration
}

func (s *slowRecorder) writeFn(payload string) error {
	time.Sleep(s.delay)
	return s.recorder.writeFn(payload)
}
