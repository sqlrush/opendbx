// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package streaming handles partial-token rendering (LLM streaming output).
// Appends tokens to an in-progress buffer, flushes a completed block on
// boundary, never re-orders already-rendered lines (防 opendb 痛点 1.1).
//
// spec-0.13 D-1 ships the Stream interface; spec-1.6 delivers the
// production TokenStream impl (see stream.go) with multi-producer chan,
// partial-line accumulator, integral fence emit, finish_reason 3-entry
// closure, thinking-only Empty placeholder (痛点 1.5), and Drain-after-
// Close ownership.
//
// DAG position: render/streaming is index 9 (true root). Imports
// render/block (RenderNode interface + Message struct). Does NOT import
// render/scrollback — caller bridges via `for _, blk := range
// stream.Drain() { sb.Push(blk) }` per spec-1.6 § 3.3 (R2 D12).
//
// Design: spec-0.13-render-engine-skeleton § 2.1 (D-1); spec-1.6-streaming-incremental.
package streaming

import (
	"github.com/sqlrush/opendbx/internal/app/cli/render/block"
)

// Stream is the partial-token appender + frame flusher.
type Stream interface {
	Append(token string)
	Flush() (block.RenderNode, error)
}
