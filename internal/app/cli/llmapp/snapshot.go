// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File snapshot.go — captures a report.RunSnapshot from the Loop's Event stream
// within a single Run (spec-1.23 D-3).
//
// Why here and not from diagnose.Result.Messages: Result.Messages is the
// provider WIRE transcript and does NOT contain the final user-visible answer
// (spec-1.21 loop.go:241). The fault report needs the answer + the submitted
// prompt + a structured 1:N tool timeline, so spec-1.23 (CRIT-1 ★B) builds a
// snapshot from the Events the Model already streams — leaving the FROZEN
// spec-1.21 Result.Messages contract untouched.
//
// Concurrency: the builder is goroutine-local to loopStartCmd's emit closure;
// only the SEALED value escapes (via the EventFinish control message), so there
// is no shared-mutable-state race (`go test -race`).

package llmapp

import (
	"strings"
	"time"

	"github.com/sqlrush/opendbx/internal/app/diagnose"
	"github.com/sqlrush/opendbx/internal/app/report"
	"github.com/sqlrush/opendbx/internal/domain/llm"
)

type snapshotBuilder struct {
	now      func() time.Time
	prompt   string
	answer   strings.Builder
	timeline []report.ToolEvent
	idIndex  map[string]int // ToolUseID → ToolTimeline index (1:N pairing)
	started  time.Time
}

func newSnapshotBuilder(prompt string, now func() time.Time) *snapshotBuilder {
	return &snapshotBuilder{
		now:     now,
		prompt:  prompt,
		idIndex: map[string]int{},
		started: now(),
	}
}

// addText accumulates a visible answer token (thinking tokens are excluded by
// the caller).
func (b *snapshotBuilder) addText(text string) { b.answer.WriteString(text) }

// addToolCall records a tool dispatch (EventToolCall), keyed by ToolUseID so a
// later result joins it even with multiple tools per turn.
func (b *snapshotBuilder) addToolCall(tu *llm.ToolUse) {
	if tu == nil {
		return
	}
	b.timeline = append(b.timeline, report.ToolEvent{Name: tu.Name, Input: tu.Input})
	b.idIndex[tu.ID] = len(b.timeline) - 1
}

// addToolResult joins a result (EventToolResult) to its dispatched call by ID.
// An unmatched result (orphan) is ignored rather than panicking.
func (b *snapshotBuilder) addToolResult(tr *llm.ToolResult, cached bool) {
	if tr == nil {
		return
	}
	if idx, ok := b.idIndex[tr.ToolUseID]; ok {
		b.timeline[idx].Result = tr.Content
		b.timeline[idx].IsError = tr.IsError
		b.timeline[idx].Cached = cached
	}
}

// seal produces the finished snapshot at EventFinish. The result is a fresh
// value safe to send over the control channel.
func (b *snapshotBuilder) seal(e diagnose.Event) *report.RunSnapshot {
	return &report.RunSnapshot{
		Prompt:       b.prompt,
		FinalAnswer:  b.answer.String(),
		ToolTimeline: b.timeline,
		Turns:        e.Turn,
		FinishStatus: e.Finish,
		TermCode:     e.TermCode,
		StartedAt:    b.started,
		FinishedAt:   b.now(),
	}
}
