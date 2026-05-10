// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package entrypoints

import (
	"context"
	"errors"
	"testing"
)

// All stub launchers must return ErrLauncherNotImplemented in stage 0.
func TestLaunchers_AllReturnNotImplemented(t *testing.T) {
	ctx := context.Background()

	t.Run("LaunchSetupDialog", func(t *testing.T) {
		s, err := LaunchSetupDialog(ctx, SetupDialogProps{})
		if !errors.Is(err, ErrLauncherNotImplemented) {
			t.Errorf("expected ErrLauncherNotImplemented, got %v", err)
		}
		if s != "" {
			t.Errorf("expected empty string, got %q", s)
		}
	})

	t.Run("LaunchInvalidSettingsDialog", func(t *testing.T) {
		err := LaunchInvalidSettingsDialog(ctx, InvalidSettingsProps{})
		if !errors.Is(err, ErrLauncherNotImplemented) {
			t.Errorf("expected ErrLauncherNotImplemented, got %v", err)
		}
	})

	t.Run("LaunchAssistantSessionChooser", func(t *testing.T) {
		s, err := LaunchAssistantSessionChooser(ctx, SessionChooserProps{})
		if !errors.Is(err, ErrLauncherNotImplemented) {
			t.Errorf("expected ErrLauncherNotImplemented, got %v", err)
		}
		if s != "" {
			t.Errorf("expected empty string, got %q", s)
		}
	})

	t.Run("LaunchAssistantInstallWizard", func(t *testing.T) {
		s, err := LaunchAssistantInstallWizard(ctx)
		if !errors.Is(err, ErrLauncherNotImplemented) {
			t.Errorf("expected ErrLauncherNotImplemented, got %v", err)
		}
		if s != "" {
			t.Errorf("expected empty string, got %q", s)
		}
	})

	t.Run("LaunchTeleportResumeWrapper", func(t *testing.T) {
		v, err := LaunchTeleportResumeWrapper(ctx)
		if !errors.Is(err, ErrLauncherNotImplemented) {
			t.Errorf("expected ErrLauncherNotImplemented, got %v", err)
		}
		if v != nil {
			t.Errorf("expected nil value, got %v", v)
		}
	})

	t.Run("LaunchTeleportRepoMismatchDialog", func(t *testing.T) {
		s, err := LaunchTeleportRepoMismatchDialog(ctx, TeleportRepoMismatchProps{})
		if !errors.Is(err, ErrLauncherNotImplemented) {
			t.Errorf("expected ErrLauncherNotImplemented, got %v", err)
		}
		if s != "" {
			t.Errorf("expected empty string, got %q", s)
		}
	})
}
