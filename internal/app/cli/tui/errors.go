// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package tui

import "github.com/sqlrush/opendbx/internal/platform/errcode"

// ErrInitFailed is returned by NewScreen when tcell.NewScreen or
// Screen.Init fails. Mapped to errcode TERMINAL.INIT_FAILED.
//
//nolint:gochecknoglobals // spec-0.6 contract: errcode sentinels are package-level.
var ErrInitFailed = errcode.Register(
	"TERMINAL.INIT_FAILED",
	"tcell screen init failed",
	"verify $TERM and that the terminal supports ANSI escape sequences",
)
