// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package bootstrap

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	tcellpkg "github.com/sqlrush/opendbx/internal/app/cli/tui"
)

// TestLaunchInteractiveTUI_NewScreenFailure exercises the init-failure
// path. Replaces the screen factory with a stub that always errors.
// spec-1.17 D-6b: error pass-through unchanged from spec-0.12.
func TestLaunchInteractiveTUI_NewScreenFailure(t *testing.T) {
	// NOT t.Parallel — mutates package-global factory state.
	orig := getNewScreenFn()
	setNewScreenFn(func() (tcell.Screen, error) {
		return nil, tcellpkg.ErrInitFailed
	})
	t.Cleanup(func() { setNewScreenFn(orig) })

	err := LaunchInteractiveTUI(context.Background())
	if !errors.Is(err, tcellpkg.ErrInitFailed) {
		t.Errorf("expected ErrInitFailed; got %v", err)
	}
}

// TestLaunchInteractiveTUI_HappyPath covers the program.Run production
// path (spec-1.17 D-6b migration from the legacy tui.Run path). The
// factory returns an un-init'd SimulationScreen; the tcell adapter's
// Driver.Init (driven by scheduler.Run) initializes it. A Ctrl+C
// double-press drives the quit protocol so Run returns nil.
//
// spec-1.17 R-6 absorb: this test was migrated from tui.Run +
// SimulationScreen to program.Run + SimulationScreen. The legacy
// tui.Run path is retired from production (no caller) but its function
// body remains for any future legacy use.
func TestLaunchInteractiveTUI_HappyPath(t *testing.T) {
	// NOT t.Parallel — mutates package-global factory state.
	orig := getNewScreenFn()
	sim := tcellpkg.NewSimulationScreen()
	// NOTE: do NOT Init the sim here — the tcell adapter's Driver.Init
	// (invoked by scheduler.Run) owns Init (spec-1.17 D-6a lifecycle).
	setNewScreenFn(func() (tcell.Screen, error) {
		return sim, nil
	})
	t.Cleanup(func() { setNewScreenFn(orig) })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Drive a Ctrl+C double-press once the screen is live. The program
	// quit protocol (spec-1.15 D-5) treats two Ctrl+C within the window
	// as quit → program.Run returns nil.
	go func() {
		time.Sleep(60 * time.Millisecond)
		sim.InjectKey(tcell.KeyCtrlC, 0, tcell.ModCtrl)
		time.Sleep(20 * time.Millisecond)
		sim.InjectKey(tcell.KeyCtrlC, 0, tcell.ModCtrl)
	}()

	if err := LaunchInteractiveTUI(ctx); err != nil {
		t.Errorf("expected nil from Ctrl+C double-press quit; got %v", err)
	}
}
