// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package bootstrap

import (
	"context"
	"sync"

	"github.com/gdamore/tcell/v2"
	"github.com/sqlrush/opendbx/internal/app/cli/tui"
)

// newScreenFn is the screen factory; production uses tui.NewScreen.
// Tests replace it with a SimulationScreen factory.
//
// T-13 go M-2: newScreenFnMu guards mutation/read in case future
// tests run in parallel inside this package. Current tests document
// "NOT t.Parallel" but the mutex makes the contract machine-enforced.
//
//nolint:gochecknoglobals // spec-0.12 D-3: test seam for SimulationScreen injection.
var (
	newScreenFnMu sync.RWMutex
	newScreenFn   = tui.NewScreen
)

// getNewScreenFn returns the current factory (read-locked).
func getNewScreenFn() func() (tcell.Screen, error) {
	newScreenFnMu.RLock()
	defer newScreenFnMu.RUnlock()
	return newScreenFn
}

// setNewScreenFn replaces the factory (write-locked). Tests call this
// via the test-only setNewScreenFn function exported within the same
// package (see tui_launcher_test.go).
func setNewScreenFn(fn func() (tcell.Screen, error)) {
	newScreenFnMu.Lock()
	defer newScreenFnMu.Unlock()
	newScreenFn = fn
}

// LaunchInteractiveTUI runs the empty tcell main loop. spec-0.12 D-4:
// cmd → entrypoints → bootstrap → app/cli/tui is the IMP layer chain;
// cmd cannot import internal/app/* directly. Layer matrix:
//   - bootstrap → app (allowed)
//   - entrypoints → bootstrap (allowed)
//   - cmd → entrypoints (allowed)
//
// Returns nil on key-exit (Ctrl+C / Ctrl+\), ctx.Err on cancel,
// ErrInitFailed wrap on tcell screen failure.
func LaunchInteractiveTUI(ctx context.Context) error {
	screen, err := getNewScreenFn()()
	if err != nil {
		// errcode-lint:exempt -- spec-0.12 D-3: err is already wrapped as TERMINAL.INIT_FAILED by tui.NewScreen; pass-through.
		return err
	}
	defer screen.Fini()
	// errcode-lint:exempt -- spec-0.12 D-3: runTUI wraps tui.Run which returns nil / ctx.Err verbatim (stdlib sentinel pass-through).
	return runTUI(ctx, screen)
}

// runTUI is the test seam between LaunchInteractiveTUI and tui.Run.
// Wrapped so the bootstrap unit test can drive tui.Run with a
// SimulationScreen (real tui.Run path execution under -race).
//
// T-13 N-3 grep aid: function exists ONLY for test injection; never
// add logic here — keep it a one-line wrapper so production behavior
// stays in tui.Run.
func runTUI(ctx context.Context, screen tcell.Screen) error {
	// errcode-lint:exempt -- spec-0.12 D-3: tui.Run returns nil / ctx.Err verbatim; stdlib sentinel pass-through, not a custom error type.
	return tui.Run(ctx, screen)
}
