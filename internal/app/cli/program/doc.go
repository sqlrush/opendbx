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
// DAG position: program is a root caller (index = 10).
// Actual imports today (R2 L-1 sweep):
//   - render/buffer / render/scheduler / render/style /
//     render/terminal / render/width
//
// Reverse imports FORBIDDEN.
//
// Forward (spec-1.16 / spec-1.17 / spec-1.20 / spec-1.21 will add):
//   - render/block (Model.View output via block render pipeline)
//   - tui (production entry replaces tui.Run with program.Run per Q13 ★A)
package program
