// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package program

import (
	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
	"github.com/sqlrush/opendbx/internal/app/cli/render/scheduler"
	"github.com/sqlrush/opendbx/internal/app/cli/render/style"
)

// Model is the spec-1.15 application state contract. Update returns a
// new Model (immutable; do not mutate the receiver). View renders the
// scrollback area only; input row and status line are produced by
// optional interfaces (InputModel, StatusSegmenter) implemented on the
// same model value.
//
// Update MUST be a pure function — no IO, no goroutine launch, no
// channel send. All side effects must flow through the returned Cmd.
type Model interface {
	Init() scheduler.Cmd
	Update(msg scheduler.Msg) (Model, scheduler.Cmd)
	View(cols, rows int) buffer.Buffer
}

// StatusSegmenter is an optional interface a Model may implement to
// drive the status line content (bottom row). Models that do not
// implement it get a fallback "opendbx" segment.
type StatusSegmenter interface {
	StatusSegments() []StatusSegment
}

// StatusSegment is one styled segment of the status line.
type StatusSegment struct {
	Text  string
	Style style.Style
}

// InputModel is an optional interface a Model may implement to drive
// the input row (above the status line). Without this, Program paints
// a static "> " prompt placeholder.
type InputModel interface {
	InputState() InputState
}

// InputState describes the current input buffer + cursor position.
type InputState struct {
	Buffer string
	Cursor int // rune position
}

// Cleanup is an optional interface a Model may implement for graceful
// shutdown side effects (close connections, flush logs). The returned
// Cmd is executed SYNCHRONOUSLY at Program.Run exit, after the
// scheduler has stopped — so the body must NOT depend on scheduler /
// worker pool availability. Returning nil is canonical when the Model
// has no cleanup work.
type Cleanup interface {
	Cleanup() scheduler.Cmd
}
