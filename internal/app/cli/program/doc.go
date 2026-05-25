// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package program is the spec-1.15 TUI application-layer skeleton: a
// Bubbletea-style Model + Update + View loop layered on top of the
// spec-1.4 FrameScheduler. The scheduler owns the single render
// goroutine; program provides the RenderFn closure and a message
// dispatch hook (WithMsgHook), so model.Update and model.View run on
// the scheduler goroutine — single-goroutine ownership of p.model,
// no atomic.Pointer or mutex needed.
//
// Design refs:
//   - spec-1.15-tui-program.md (DRAFT R5)
//   - spec-1.4 R3 errata (scheduler.Cmd / Msg / WithMsgHook / EmitMsg / EmitError)
//
// DAG position: program is a root caller (index = 10) — imports
// render/buffer, render/scheduler, render/style, render/terminal,
// render/width, render/block, tui. Reverse imports FORBIDDEN.
package program
