// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package tablerun

import "testing"

// Skippable lets a case opt into being skipped by returning a non-empty
// reason from SkipReason(). The case is not even sub-Run when skipped.
// spec-0.11 R2 codex MED-1: explicit interface beats untyped reflective
// field lookup.
type Skippable interface {
	SkipReason() string
}

// Run executes table-driven cases SERIALLY (no t.Parallel) by default.
// Each case T MUST have an exported `Name string` field; missing or
// empty Name calls t.Fatalf.
//
// If T also implements Skippable and SkipReason() returns a non-empty
// string, the case is skipped via t.Skip.
//
// spec-0.11 D-1. See package doc for rationale on serial default.
func Run[T any](t *testing.T, cases []T, fn func(t *testing.T, c T)) {
	t.Helper()
	for i, c := range cases {
		c, i := c, i
		name := mustExtractName(t, i, c)
		t.Run(name, func(t *testing.T) {
			if s, ok := any(c).(Skippable); ok {
				if reason := s.SkipReason(); reason != "" {
					t.Skip(reason)
					return
				}
			}
			fn(t, c)
		})
	}
}

// RunParallel executes cases with t.Parallel(). Use only when caller
// has audited that no case mutates process-global state (no t.Setenv,
// no os.Chdir, no package-level vars).
//
// Other contract identical to Run.
func RunParallel[T any](t *testing.T, cases []T, fn func(t *testing.T, c T)) {
	t.Helper()
	for i, c := range cases {
		c, i := c, i
		name := mustExtractName(t, i, c)
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if s, ok := any(c).(Skippable); ok {
				if reason := s.SkipReason(); reason != "" {
					t.Skip(reason)
					return
				}
			}
			fn(t, c)
		})
	}
}
