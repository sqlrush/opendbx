// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package scheduler is the Bubbletea-like Cmd tick + time-slicing
// scheduler that drives the render loop. spec-0.13 D-1 ships interface
// only; spec-1.4 fills the algorithm.
//
// DAG position: render/scheduler is index 6 (depends on render/optimizer
// + render/terminal).
//
// Design: spec-0.13-render-engine-skeleton § 2.1 (D-1)
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
