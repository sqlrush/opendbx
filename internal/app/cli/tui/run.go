// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package tui

import (
	"context"
	"sync"

	"github.com/gdamore/tcell/v2"
	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

// Test seams for NewScreen error branches. Production values stay as direct
// tcell constructors; tests temporarily replace them without exposing tcell
// outside this package.
//
//nolint:gochecknoglobals // spec-0.12 D-3: constructor seams for terminal init failure tests.
var (
	newStdIoTtyFn              = tcell.NewStdIoTty
	newTerminfoScreenFromTtyFn = tcell.NewTerminfoScreenFromTty
)

// NewScreen creates and initializes a real tcell screen bound to the
// current process stdin/stdout. We intentionally avoid tcell.NewScreen()
// because it defaults to /dev/tty on POSIX; PTY-backed E2E tests and some
// launcher contexts do not permit opening /dev/tty even though stdin/stdout
// are valid TTYs.
//
// Wraps init failures as ErrInitFailed so callers (cmd/opendbx) don't need
// to import tcell — preserves IMP-9 tcell isolation. Layer chain:
// cmd→entrypoints→bootstrap→tui; the 3 packages whitelisted for tcell
// production imports are terminal / tui / bootstrap (T-13 L-5
// reconciliation of R-13 spec letter vs impl).
func NewScreen() (tcell.Screen, error) {
	tty, err := newStdIoTtyFn()
	if err != nil {
		// T-13 N-2: non-empty Hint so users have actionable guidance.
		return nil, errcode.Wrap("TERMINAL.INIT_FAILED", err,
			"tcell.NewStdIoTty failed",
			"verify $TERM is set and the terminal supports ANSI escape sequences (e.g. xterm-256color)")
	}
	screen, err := newTerminfoScreenFromTtyFn(tty)
	if err != nil {
		if tty != nil {
			_ = tty.Close()
		}
		return nil, errcode.Wrap("TERMINAL.INIT_FAILED", err,
			"tcell.NewTerminfoScreenFromTty failed",
			"verify $TERM is set and the terminal supports ANSI escape sequences (e.g. xterm-256color)")
	}
	if err := screen.Init(); err != nil {
		return nil, errcode.Wrap("TERMINAL.INIT_FAILED", err,
			"tcell.Screen.Init failed",
			"check $TERM and that stdout is a real terminal; running under sudo / inside CI may strip TTY access")
	}
	return screen, nil
}

// NewSimulationScreen creates a tcell SimulationScreen for tests.
// Same factory pattern as NewScreen so test callers also avoid
// importing tcell from non-whitelisted packages.
//
// Note: SimulationScreen returned without calling Init — caller must
// Init() it explicitly before passing to Run (matches real-Screen
// contract where caller drives Init).
func NewSimulationScreen() tcell.SimulationScreen {
	return tcell.NewSimulationScreen("UTF-8")
}

// Run runs the empty tcell main loop until exit. Exits on:
//   - tcell.EventKey with KeyCtrlC or KeyCtrlBackslash → returns nil
//   - context cancellation → returns ctx.Err()
//
// Caller MUST provide an already-Init'd screen. Caller MUST defer
// screen.Fini() to restore terminal state.
//
// Internally spawns one goroutine to bridge ctx.Done() →
// screen.PostEvent(NewEventInterrupt) since PollEvent is blocking.
// Goroutine is joined via WaitGroup before Run returns; panic path
// also joins via defer.
//
// spec-0.12 R2 H-1 + R3 MED-1.
func Run(ctx context.Context, screen tcell.Screen) error {
	if screen == nil {
		return ErrInitFailed
	}

	// Initial empty frame.
	screen.Clear()
	screen.Show()

	// goroutine bridge ctx → PostEvent. Done channel + WaitGroup
	// guarantee no leak on panic / normal-exit / cancel paths.
	var wg sync.WaitGroup
	done := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
			// PostEvent thread-safe; ignore err (screen Fini'd is harmless).
			_ = screen.PostEvent(tcell.NewEventInterrupt(nil))
		case <-done:
		}
	}()
	defer func() {
		close(done)
		wg.Wait()
	}()

	for {
		ev := screen.PollEvent()
		if ev == nil {
			// Screen was Fini'd externally — exit cleanly.
			return nil
		}
		switch e := ev.(type) {
		case *tcell.EventKey:
			switch e.Key() {
			case tcell.KeyCtrlC, tcell.KeyCtrlBackslash:
				return nil
			}
			// Other keys ignored (no input buffer at Stage 0; spec-1.15 wires dispatcher).
		case *tcell.EventResize:
			screen.Sync()
		case *tcell.EventInterrupt:
			// errcode-lint:exempt -- spec-0.12 D-3: ctx.Err returns context.Canceled / DeadlineExceeded verbatim by design; cmd/opendbx layer maps ctx errors.
			if err := ctx.Err(); err != nil {
				return err
			}
			// Spurious interrupt — return to PollEvent.
		}
	}
}
