// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Stage-0 dialog launcher stubs (spec-0.3 D-5).
//
// These mirror the 5+ launchers in CC src/dialogLaunchers.tsx — see file
// header comments for site references. Each launcher:
//
//   - takes a context.Context (replaces CC's React Root + props pattern with
//     idiomatic Go cancellation)
//   - takes a typed *Props struct (params equivalent to CC component Props)
//   - returns the same "shape" CC's Promise<T> resolves to (Go: typed value
//     + error)
//
// Stage-0 every launcher returns ErrLauncherNotImplemented. spec-1.15-tui
// replaces these with real React/Ink-equivalent implementations driven by the
// internal/app/cli/render engine.

package entrypoints

import (
	"context"
	"errors"
)

// ErrLauncherNotImplemented signals a dialog launcher has not yet been wired
// to a real React/Ink-equivalent UI.
var ErrLauncherNotImplemented = errors.New("dialog launcher not implemented in stage 0 (lands in spec-1.15-tui)")

// SetupDialogProps parallels CC `<SnapshotUpdateDialog>` props.
type SetupDialogProps struct {
	AgentType         string
	Scope             string // "global" / "project" / "local" (mirrors CC AgentMemoryScope)
	SnapshotTimestamp string
}

// LaunchSetupDialog parallels CC dialogLaunchers.tsx::launchSnapshotUpdateDialog
// (site ~3173). Returns one of "merge" / "keep" / "replace".
func LaunchSetupDialog(_ context.Context, _ SetupDialogProps) (string, error) {
	return "", ErrLauncherNotImplemented
}

// InvalidSettingsProps parallels CC `<InvalidSettingsDialog>` props.
type InvalidSettingsProps struct {
	SettingsErrors []string // mirrors CC ValidationError[]
	OnExit         func()
}

// LaunchInvalidSettingsDialog parallels CC site ~3250. CC version returns
// void; opendbx version returns error to surface dialog dispatch failures.
func LaunchInvalidSettingsDialog(_ context.Context, _ InvalidSettingsProps) error {
	return ErrLauncherNotImplemented
}

// SessionChooserProps parallels CC `<AssistantSessionChooser>` props.
type SessionChooserProps struct {
	Sessions []AssistantSession
}

// AssistantSession mirrors CC AssistantSession.
type AssistantSession struct {
	ID    string
	Title string
}

// LaunchAssistantSessionChooser parallels CC site ~4229. Returns the chosen
// session ID or "" if cancelled.
func LaunchAssistantSessionChooser(_ context.Context, _ SessionChooserProps) (string, error) {
	return "", ErrLauncherNotImplemented
}

// LaunchAssistantInstallWizard parallels CC NewInstallWizard launcher. Returns
// the chosen install dir or "" if cancelled. Returns error on installation
// failure (per CC contract).
func LaunchAssistantInstallWizard(_ context.Context) (string, error) {
	return "", ErrLauncherNotImplemented
}

// TeleportRemoteResponse mirrors CC TeleportRemoteResponse. Stage-0 stub uses
// any to avoid binding to spec-2.6's eventual remote-session schema.
type TeleportRemoteResponse = any

// LaunchTeleportResumeWrapper parallels CC site ~4549.
func LaunchTeleportResumeWrapper(_ context.Context) (TeleportRemoteResponse, error) {
	return nil, ErrLauncherNotImplemented
}

// TeleportRepoMismatchProps parallels CC `<TeleportRepoMismatchDialog>` props.
type TeleportRepoMismatchProps struct {
	ExpectedRepo string
	LocalPaths   []string
}

// LaunchTeleportRepoMismatchDialog parallels CC site ~4597. Returns the
// chosen local path or "" if cancelled.
func LaunchTeleportRepoMismatchDialog(_ context.Context, _ TeleportRepoMismatchProps) (string, error) {
	return "", ErrLauncherNotImplemented
}
