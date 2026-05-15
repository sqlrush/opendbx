// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

//go:build !windows

package uitest

import (
	"bytes"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestANSIRaw_RoundTrip launches a helper child that emits known ANSI
// then asserts ANSIRaw returns the byte sequence (or a superset
// including the banner).
func TestANSIRaw_RoundTrip(t *testing.T) {
	term := Term(t, helperCmd(t, "banner"), 80, 24)
	term.WaitFor(t, func(*Terminal) bool {
		return strings.Contains(strings.Join(term.CellGrid(), "\n"), "ready")
	}, time.Second)
	raw, err := term.ANSIRaw()
	if err != nil {
		t.Fatalf("ANSIRaw: %v", err)
	}
	if !bytes.Contains(raw, []byte("> ready")) {
		t.Errorf("ANSIRaw should contain '> ready'; got %q", raw)
	}
}

// TestANSIRaw_Concurrent stress-tests concurrent ANSIRaw reads with
// pumpOutput writes — race detector must report 0 races over many
// iterations.
func TestANSIRaw_Concurrent(t *testing.T) {
	term := Term(t, helperCmd(t, "banner"), 80, 24)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_, _ = term.ANSIRaw()
			}
		}()
	}
	// Wait for child banner emission to complete (so all goroutines
	// see at least some captured bytes).
	term.WaitFor(t, func(*Terminal) bool {
		return strings.Contains(strings.Join(term.CellGrid(), "\n"), "ready")
	}, time.Second)
	wg.Wait()
}

// TestANSIRaw_OverflowCap synthesizes a Terminal struct directly and
// invokes captureWriter.Write with payloads exceeding 10 MiB to
// verify cap + ErrAnsiBufFull behavior.
func TestANSIRaw_OverflowCap(t *testing.T) {
	// Build a Terminal without going through Term() (no real PTY).
	term := &Terminal{}
	cw := &captureWriter{term: term}

	// Write just below cap.
	belowCap := make([]byte, maxANSIBufBytes-100)
	n, err := cw.Write(belowCap)
	if err != nil || n != len(belowCap) {
		t.Fatalf("Write below cap: n=%d err=%v", n, err)
	}
	got, err := term.ANSIRaw()
	if err != nil {
		t.Fatalf("ANSIRaw below cap should be err-free: %v", err)
	}
	if len(got) != len(belowCap) {
		t.Errorf("expected %d bytes, got %d", len(belowCap), len(got))
	}

	// Write 200 more bytes — first 100 fits in cap, next 100 overflow.
	more := make([]byte, 200)
	for i := range more {
		more[i] = byte('x')
	}
	n, err = cw.Write(more)
	if err != nil || n != len(more) {
		t.Fatalf("Write across cap: n=%d err=%v", n, err)
	}
	got, err = term.ANSIRaw()
	if !errors.Is(err, ErrAnsiBufFull) {
		t.Fatalf("expected ErrAnsiBufFull; got %v", err)
	}
	if len(got) != maxANSIBufBytes {
		t.Errorf("buffer should be capped at %d; got %d", maxANSIBufBytes, len(got))
	}

	// Further Write should be no-op + still report overflow.
	n, err = cw.Write([]byte("more"))
	if err != nil || n != 4 {
		t.Fatalf("Write after cap: n=%d err=%v", n, err)
	}
	got, err = term.ANSIRaw()
	if !errors.Is(err, ErrAnsiBufFull) {
		t.Error("ErrAnsiBufFull should remain sticky")
	}
	if len(got) != maxANSIBufBytes {
		t.Errorf("buffer should not grow past cap; got %d", len(got))
	}
}
