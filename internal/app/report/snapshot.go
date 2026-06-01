// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File snapshot.go — RunSnapshot: the report's deterministic input (spec-1.23
// D-3). It is the structured, llmapp-captured record of a single completed
// diagnosis run.
//
// Why NOT diagnose.Result.Messages: that field is the provider WIRE transcript
// (spec-1.21) and intentionally does NOT contain the final user-visible answer
// — Loop.Run returns on FinishStop without committing the terminal assistant
// text to Messages (loop.go:241; locked by loop_integration_test.go:146). The
// fault report needs the final answer, the submitted prompt, and a structured
// tool timeline, so spec-1.23 (CRIT-1 ★B) captures a RunSnapshot in llmapp from
// the Event stream rather than re-deriving it from the wire transcript. This
// keeps the FROZEN spec-1.21 Result.Messages contract untouched.

package report

import (
	"time"

	"github.com/sqlrush/opendbx/internal/domain/llm"
)

// ToolEvent is one tool round-trip in a run (spec-1.21 EventToolCall paired
// with its EventToolResult; supports the 1:N multi-tool-per-turn shape).
type ToolEvent struct {
	Name    string
	Input   map[string]any // rendered via canonical (key-sorted) JSON — never map-iterated for output
	Result  string         // tool_result content (summarized at render time)
	IsError bool
	Cached  bool // spec-1.22 dedup hit
}

// RunSnapshot is the sealed record of one diagnosis run, captured by llmapp and
// consumed by Generate. It is a pure value: Generate(snapshot, now) is a
// deterministic function (no time.Now, no map iteration for output).
type RunSnapshot struct {
	Prompt       string           // the current run's user question (captured at submit, NOT guessed from history)
	FinalAnswer  string           // the final visible answer, accumulated from EventText(visible)
	ToolTimeline []ToolEvent      // EventToolCall/EventToolResult order
	Turns        int              // diagnose.Result.Turns
	FinishStatus llm.FinishReason // terminal finish reason
	TermCode     string           // DIAGNOSE.* code ("" on natural FinishStop)
	StartedAt    time.Time        // run start (submit)
	FinishedAt   time.Time        // run seal (EventFinish)
}
