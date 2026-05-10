// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package profileutil records timestamped startup checkpoints (spec-0.3 D-9).
//
// Stage-0 stub: parallels CC main.tsx L1 `profileCheckpoint('main_tsx_entry')`.
// Records (name → time.Time) into a sync.Map; Report() prints the durations.
// Real profiler (lock-free buffer or chan, possibly chrome-trace JSON output)
// lands in spec-3.X-perf.
//
// Per spec-0.3 § 1.4 + spec-0.2 § 2.2 cmd→platform/version唯一例外: spec-0.3
// implicitly **lifts** profileutil into the cmd→platform allow list because
// `cmd/opendbx/main.go` calls `profileutil.Checkpoint("main_entry")` before
// any other code can run (entrypoints/bootstrap not yet alive).
//
// This is the second cmd→platform exception (after platform/version). It will
// be formalized in the spec-0.3 PR by amending § 2.2 of spec-0.2 (or by
// landing the change in opendbx/tools/import-rules-check/rules/layers.go's
// CmdPlatformExceptions list).
package profileutil

import (
	"fmt"
	"io"
	"sort"
	"sync"
	"time"
)

// store keeps insertion order for deterministic Report output.
type entry struct {
	name string
	at   time.Time
}

var (
	mu        sync.Mutex
	entries   []entry
	startOnce sync.Once
	start     time.Time
)

// Checkpoint records the time `name` was reached. Multiple checkpoints with
// the same name are kept (caller's responsibility to use unique names).
func Checkpoint(name string) {
	mu.Lock()
	defer mu.Unlock()
	startOnce.Do(func() {
		start = time.Now()
	})
	entries = append(entries, entry{name: name, at: time.Now()})
}

// Report writes a deterministic summary of recorded checkpoints to w.
// Format:
//
//	[profile]    0.0ms  main_entry
//	[profile]    1.2ms  parsing_flags
//	[profile]    2.7ms  cobra_execute_start
//	[profile] N=3 total=2.7ms
//
// Returns immediately if no checkpoints were recorded.
func Report(w io.Writer) {
	mu.Lock()
	defer mu.Unlock()
	if len(entries) == 0 {
		return
	}
	// Sort by time (stable since all entries from same process should be
	// monotonically increasing under wall clock).
	cp := make([]entry, len(entries))
	copy(cp, entries)
	sort.SliceStable(cp, func(i, j int) bool { return cp[i].at.Before(cp[j].at) })

	for _, e := range cp {
		dt := e.at.Sub(start).Seconds() * 1000
		_, _ = fmt.Fprintf(w, "[profile] %7.1fms  %s\n", dt, e.name)
	}
	if last := cp[len(cp)-1]; len(cp) > 0 {
		_, _ = fmt.Fprintf(w, "[profile] N=%d total=%.1fms\n",
			len(cp), last.at.Sub(start).Seconds()*1000)
	}
}

// Reset clears all recorded checkpoints. Useful in tests.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	entries = nil
	start = time.Time{}
	startOnce = sync.Once{}
}
