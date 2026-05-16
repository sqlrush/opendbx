// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package streaming handles partial-token rendering (LLM streaming output).
// Appends tokens to an in-progress buffer, flushes a completed block on
// boundary, never re-orders already-rendered lines (防 opendb 痛点 1.1).
// spec-0.13 D-1 ships interface only; the real implementation lands in
// spec-2.x streaming-token-handling.
//
// DAG position: render/streaming is index 9 (true root; depends on
// render/scrollback + render/block).
//
// Design: spec-0.13-render-engine-skeleton § 2.1 (D-1)
package streaming

import (
	"github.com/sqlrush/opendbx/internal/app/cli/render/block"
)

// Stream is the partial-token appender + frame flusher.
type Stream interface {
	Append(token string)
	Flush() (block.RenderNode, error)
}
