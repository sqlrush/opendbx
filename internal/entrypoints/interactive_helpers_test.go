// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package entrypoints

import (
	"context"
	"errors"
	"testing"
)

// spec-0.12 D-4: RenderAndRun replaced the spec-0.3 stub with a real
// tui launcher (bootstrap.LaunchInteractiveTUI). In a non-TTY test
// environment tcell.Screen.Init fails → wrapped as TERMINAL.INIT_FAILED.
// The "not implemented" sentinel is now only for ShowSetupDialog.
func TestRenderAndRun_NoTTY(t *testing.T) {
	t.Setenv("TERM", "")
	err := RenderAndRun(context.Background())
	if err == nil {
		t.Skip("test runner provided a usable tcell screen (rare); skip non-TTY assertion")
		return
	}
	if errors.Is(err, ErrInteractiveHelperNotImplemented) {
		t.Errorf("RenderAndRun should no longer return ErrInteractiveHelperNotImplemented; got %v", err)
	}
}

func TestShowSetupDialog_NotImplemented(t *testing.T) {
	v, err := ShowSetupDialog(context.Background(), nil)
	if !errors.Is(err, ErrInteractiveHelperNotImplemented) {
		t.Errorf("expected ErrInteractiveHelperNotImplemented, got %v", err)
	}
	if v != nil {
		t.Errorf("expected nil value, got %v", v)
	}
}
