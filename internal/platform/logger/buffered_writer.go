// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package logger

import (
	"errors"
	"math"
	"strings"
	"sync"
	"time"
)

// ErrWriterClosed is returned by bufferedWriter.Write after Dispose.
var ErrWriterClosed = errors.New("logger: BufferedWriter closed")

// writeFunc is the lowest-level I/O callback invoked by a bufferedWriter to
// flush a single batched payload. Returning an error is best-effort: the
// writer logs it via dispose contract but does not retry.
type writeFunc func(string) error

// bufferedWriterConfig holds the knobs that drive batching, flush cadence,
// and dispatch mode. CC parity defaults (Q5 ★A, hardcoded):
//
//	maxBufferSize  = 100
//	maxBufferBytes = math.MaxInt (effectively Infinity, CC parity)
//	flushInterval  = 1000ms
//	immediateMode  = false
type bufferedWriterConfig struct {
	writeFn        writeFunc
	flushInterval  time.Duration
	maxBufferSize  int
	maxBufferBytes int

	// immediateMode bypasses the buffer entirely — every Write call invokes
	// writeFn synchronously. CC uses this to avoid buffering during debug
	// mode for ants (debug.ts uses createBufferedWriter with immediateMode=
	// true in the debug code path). spec-0.5 default is false; tests may
	// flip it for deterministic assertions without timer races.
	immediateMode bool
}

// defaultBufferedWriterConfig returns the CC 1:1 defaults.
func defaultBufferedWriterConfig(fn writeFunc) bufferedWriterConfig {
	return bufferedWriterConfig{
		writeFn:        fn,
		flushInterval:  1000 * time.Millisecond,
		maxBufferSize:  100,
		maxBufferBytes: math.MaxInt,
		immediateMode:  false,
	}
}

// bufferedWriter is a Go translation of CC's createBufferedWriter
// (bufferedWriter.ts:1-100). Semantic parity:
//
//   - normal Write enqueues into an in-memory slice; the slice is drained by
//     a one-shot timer (flushInterval) or when size/byte thresholds trip.
//   - overflow detaches the buffer into pendingOverflow and launches a single
//     goroutine to drain it (CC's setImmediate equivalent), preserving
//     ordering via coalescing — subsequent overflows under the same in-flight
//     drain append to pendingOverflow rather than spawning more goroutines.
//   - Flush writes pendingOverflow + buffer in order and clears state.
//   - Dispose marks the writer closed, runs a final Flush, and Wait()s for
//     any in-flight drain goroutine.
//
// dispose contract (spec § 3, claude HIGH-4): Dispose does NOT early-return
// on flush error. Callers (impl.close in T-7) combine main + sidecar errors
// via errors.Join.
type bufferedWriter struct {
	cfg bufferedWriterConfig

	mu              sync.Mutex
	buffer          []string
	bufferBytes     int
	timer           *time.Timer
	pendingOverflow []string
	closed          bool
	overflowWG      sync.WaitGroup
}

// newBufferedWriter constructs a bufferedWriter, normalising config zero
// values to CC defaults.
func newBufferedWriter(cfg bufferedWriterConfig) *bufferedWriter {
	if cfg.flushInterval <= 0 {
		cfg.flushInterval = 1000 * time.Millisecond
	}
	if cfg.maxBufferSize <= 0 {
		cfg.maxBufferSize = 100
	}
	if cfg.maxBufferBytes <= 0 {
		cfg.maxBufferBytes = math.MaxInt
	}
	return &bufferedWriter{cfg: cfg}
}

// Write enqueues content for batched output. Returns ErrWriterClosed if
// Dispose has been called.
//
// In immediateMode, Write invokes writeFn synchronously and bypasses every
// buffer / timer / overflow path.
func (b *bufferedWriter) Write(content string) error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return ErrWriterClosed
	}
	if b.cfg.immediateMode {
		b.mu.Unlock()
		return b.cfg.writeFn(content)
	}
	defer b.mu.Unlock()
	b.buffer = append(b.buffer, content)
	b.bufferBytes += len(content)
	b.scheduleFlushLocked()
	if len(b.buffer) >= b.cfg.maxBufferSize || b.bufferBytes >= b.cfg.maxBufferBytes {
		b.flushDeferredLocked()
	}
	return nil
}

// Flush synchronously drains pendingOverflow + buffer. It is safe to call
// concurrently with Write; the in-flight drain goroutine (if any) will not
// double-write because flushLocked nils out pendingOverflow before
// drainPendingOverflow reads it under the same lock.
func (b *bufferedWriter) Flush() error {
	b.mu.Lock()
	err := b.flushLocked()
	b.mu.Unlock()
	return err
}

// flushLocked writes pendingOverflow then buffer in order. Caller must hold
// b.mu. Combined errors are NOT joined; the first error wins (Flush is
// best-effort, intermediate writes still happen).
func (b *bufferedWriter) flushLocked() error {
	var firstErr error
	if len(b.pendingOverflow) > 0 {
		if err := b.cfg.writeFn(strings.Join(b.pendingOverflow, "")); err != nil && firstErr == nil {
			firstErr = err
		}
		b.pendingOverflow = nil
	}
	if len(b.buffer) > 0 {
		if err := b.cfg.writeFn(strings.Join(b.buffer, "")); err != nil && firstErr == nil {
			firstErr = err
		}
		b.buffer = nil
		b.bufferBytes = 0
	}
	b.stopTimerLocked()
	return firstErr
}

// scheduleFlushLocked arms the one-shot drain timer. Idempotent: if a timer
// is already armed it is left untouched (CC setTimeout semantics: at most
// one timer per writer).
func (b *bufferedWriter) scheduleFlushLocked() {
	if b.timer != nil {
		return
	}
	b.timer = time.AfterFunc(b.cfg.flushInterval, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if b.closed {
			return // Dispose already flushed; nothing to do.
		}
		_ = b.flushLocked() // best-effort; the timer fire path has no caller to surface errors to.
	})
}

// stopTimerLocked cancels the active timer (if any). Caller must hold b.mu.
func (b *bufferedWriter) stopTimerLocked() {
	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}
}

// flushDeferredLocked detaches the buffer into pendingOverflow and (if none
// in flight) launches an async drain goroutine. Caller must hold b.mu.
//
// Coalescing semantics (CC parity): if pendingOverflow is non-nil (an earlier
// detached batch has not been written yet), the current buffer is APPENDED
// rather than spawning another goroutine. This bounds goroutine count to one
// in-flight overflow drain per writer and preserves ordering.
func (b *bufferedWriter) flushDeferredLocked() {
	if b.pendingOverflow != nil {
		b.pendingOverflow = append(b.pendingOverflow, b.buffer...)
		b.buffer = nil
		b.bufferBytes = 0
		b.stopTimerLocked()
		return
	}
	detached := b.buffer
	b.buffer = nil
	b.bufferBytes = 0
	b.stopTimerLocked()
	b.pendingOverflow = detached
	b.overflowWG.Add(1)
	go b.drainPendingOverflow()
}

// drainPendingOverflow runs in its own goroutine and loops until no more
// overflow batches arrive. Inside the loop we:
//
//  1. Capture pendingOverflow under the lock (any later Writes that
//     coalesced into the slice are picked up).
//  2. Release the lock and perform the actual writeFn (slow file I/O).
//  3. Re-acquire and check whether another overflow accumulated WHILE we
//     were writing. If so, keep draining.
//
// This loop closes a race window where a second overflow could otherwise
// observe pendingOverflow==nil and spawn a new goroutine — breaking
// ordering and CC parity (codex HIGH-1).
func (b *bufferedWriter) drainPendingOverflow() {
	defer b.overflowWG.Done()
	for {
		b.mu.Lock()
		toWrite := b.pendingOverflow
		if len(toWrite) == 0 {
			// Don't nil-out until we know no more work arrived — but also
			// don't leave the slice live, or other paths assume a drain is
			// already scheduled and never queue a new one. The "owner" of
			// pendingOverflow is whoever holds the goroutine slot via
			// overflowWG; we relinquish it here.
			b.pendingOverflow = nil
			b.mu.Unlock()
			return
		}
		// Keep pendingOverflow set to a non-nil sentinel so concurrent
		// flushDeferredLocked() callers see "drain in flight" and coalesce
		// rather than launching another goroutine. We swap to an empty
		// slice (still non-nil) and write the captured batch.
		b.pendingOverflow = make([]string, 0, 4)
		b.mu.Unlock()

		_ = b.cfg.writeFn(strings.Join(toWrite, ""))
		// Loop back: if other writes coalesced into pendingOverflow while
		// writeFn was running, drain them in this same goroutine to
		// preserve order.
	}
}

// Dispose marks the writer closed, flushes synchronously, and waits for any
// in-flight overflow goroutine to drain.
//
// Returns the final flush error (if any). Idempotent: second call returns nil.
func (b *bufferedWriter) Dispose() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	err := b.flushLocked()
	b.mu.Unlock()
	// Wait outside the lock — drainPendingOverflow re-acquires it.
	b.overflowWG.Wait()
	return err
}
