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
func TestLaunchInteractiveTUI_NewScreenFailure(t *testing.T) {
	// NOT t.Parallel — mutates newScreenFn package global.
	orig := newScreenFn
	newScreenFn = func() (tcell.Screen, error) {
		return nil, tcellpkg.ErrInitFailed
	}
	t.Cleanup(func() { newScreenFn = orig })

	err := LaunchInteractiveTUI(context.Background())
	if !errors.Is(err, tcellpkg.ErrInitFailed) {
		t.Errorf("expected ErrInitFailed; got %v", err)
	}
}

// TestLaunchInteractiveTUI_HappyPath covers the happy path with a
// SimulationScreen and a Ctrl+C key injection that drives tui.Run to
// return nil.
func TestLaunchInteractiveTUI_HappyPath(t *testing.T) {
	// NOT t.Parallel — mutates newScreenFn package global.
	orig := newScreenFn
	sim := tcellpkg.NewSimulationScreen()
	if err := sim.Init(); err != nil {
		t.Fatalf("SimulationScreen.Init: %v", err)
	}
	newScreenFn = func() (tcell.Screen, error) {
		return sim, nil
	}
	t.Cleanup(func() { newScreenFn = orig })

	go func() {
		time.Sleep(30 * time.Millisecond)
		sim.InjectKey(tcell.KeyCtrlC, 0, tcell.ModNone)
	}()
	if err := LaunchInteractiveTUI(context.Background()); err != nil {
		t.Errorf("expected nil from Ctrl+C exit; got %v", err)
	}
}
