// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package terminal

import "github.com/sqlrush/opendbx/internal/platform/errcode"

// ErrProbeFailed is returned by Probe when capability probing
// fundamentally fails (e.g. $TERM unset on Linux with no fallback).
// Mapped to errcode TERMINAL.PROBE_FAILED per spec-0.6 D-4.
//
//nolint:gochecknoglobals // spec-0.6 contract: errcode sentinels are package-level.
var ErrProbeFailed = errcode.Register(
	"TERMINAL.PROBE_FAILED",
	"Terminal capability probe failed",
	"verify $TERM is set and terminfo is installed; export TERM=xterm-256color and retry",
)

// ErrNotATTY is returned by callers (cmd/opendbx runInteractRoot) when
// stdin or stdout is not a TTY but interact mode requires both.
// Mapped to errcode TERMINAL.NOT_A_TTY. spec-0.12 R2 M-7 + R3 M-7 user ★.
//
//nolint:gochecknoglobals // spec-0.6 contract: errcode sentinels are package-level.
var ErrNotATTY = errcode.Register(
	"TERMINAL.NOT_A_TTY",
	"opendbx interact requires a TTY on both stdin and stdout",
	"run from a real terminal; for non-interactive use, run `opendbx <subcommand>` directly",
)
