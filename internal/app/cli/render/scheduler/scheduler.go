// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package scheduler drives the production render frame loop: a
// single-goroutine main loop ticking at a configurable FPS, fed by a
// 3-lane priority FIFO queue, with cooperating N=4 worker pool for
// off-thread Cmd execution. spec-0.13 D-1 shipped the Scheduler /
// Cmd / Tick interface skeleton; spec-1.4 fills FrameScheduler.
//
// Design (Bubbletea-like, but no Model/Update/View abstraction):
//
//   - Caller provides a RenderFn closure that populates the
//     freshly-acquired next *buffer.Grid each frame. RenderFn runs
//     ONLY on the main loop goroutine.
//   - Worker pool runs Cmds asynchronously and reports recovered
//     panics back to the main loop via a results channel.
//   - Main loop applies optimizer Patches to the terminal Driver,
//     emits Tick on success, ErrorMsg on Cmd panic.
//
// DAG position: render/scheduler is index 6 (imports render/buffer,
// render/optimizer, render/terminal, render/style; NOT block,
// scrollback, streaming, layout).
//
// Design: spec-0.13-render-engine-skeleton § 2.1 (D-1) +
// spec-1.4-scheduler-time-slice (FrameScheduler impl)
package scheduler

import "time"

// Cmd is the unit of work submitted to the scheduler.
type Cmd func()

// Tick is a frame budget signal.
type Tick struct {
	When  time.Time
	Frame int
}

// Scheduler coordinates Cmd execution and frame ticks.
type Scheduler interface {
	Schedule(cmd Cmd)
	Tick() <-chan Tick
}
