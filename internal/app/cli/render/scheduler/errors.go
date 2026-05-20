// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package scheduler

import "github.com/sqlrush/opendbx/internal/platform/errcode"

// ErrPanicRecovered is the errcode sentinel emitted (via ErrorMsg on
// FrameScheduler.Msgs) when a worker pool Cmd panics and is recovered.
//
// spec-1.4 R2 D-3 + R-2 + Q8 ★A: single Cmd panic must not take down
// the whole render loop. The opendb E11 lesson is explicit — one
// background DB query bug should never crash the TUI. CLAUDE rule 7
// 错误三件套.
//
//nolint:gochecknoglobals // spec-0.6 contract: errcode sentinels are package-level.
var ErrPanicRecovered = errcode.Register(
	"RENDER.SCHEDULER_PANIC_RECOVERED",
	"scheduled Cmd panicked and was recovered",
	"check ErrorMsg.Stack for the panic site; verify Cmd doesn't access non-thread-safe state shared with main loop; long-running Cmd should honor ctx cancellation",
)
