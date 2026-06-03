// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package llmapp

import (
	"github.com/sqlrush/opendbx/internal/app/report"
	"github.com/sqlrush/opendbx/internal/domain/llm"
)

// streamControlMsg is the control-bypass Msg (spec-1.20 R2 CRIT-2 base;
// spec-1.21.1 sealed-segment extension). Routing is by which optional
// field is set:
//
//   - PreviewTick         → EventText visible preview wakeup; Update drains
//     TokenStream into Model.previewNodes. It carries no text payload and
//     does not affect sawContent / finish semantics.
//   - SealedText != ""    → authoritative assistant text segment; appended
//     to scrollback through the same ctrl FIFO as tool events.
//   - ToolUse != nil      → EventToolCall variant
//   - ToolResult != nil   → EventToolResult variant
//   - Finish.Terminal()   → EventFinish variant (TermCode optional DIAGNOSE.*)
//   - Thinking=true       → EventText thinking-channel variant; ThinkingToken
//     carries renderable thinking only when strip_think=false. Thinking never
//     enters the TokenStream / main content plane.
//
// At most one variant is populated per message, except SealedTruncated which
// qualifies SealedText. Update dispatches variants explicitly so future
// additive fields cannot silently fall through.
type streamControlMsg struct {
	PreviewTick     bool
	SealedText      string
	SealedTruncated bool
	Thinking        bool
	ThinkingToken   string          // EventText thinking side channel (spec-1.20.2 D-5)
	ToolUse         *llm.ToolUse    // EventToolCall (spec-1.21 D-6)
	ToolResult      *llm.ToolResult // EventToolResult (spec-1.21 D-6)
	Cached          bool            // EventToolResult — spec-1.22: result served from dedup cache (render-only; ToolResult content is byte-identical to a fresh run)
	Finish          llm.FinishReason
	TermCode        string // EventFinish — DIAGNOSE.* code or "" on natural Stop
	Err             error
	Snapshot        *report.RunSnapshot // EventFinish — spec-1.23 D-3: the run snapshot, sealed atomically with the finish (rides this msg → no post-Run send → no channel-close race)
}

// readControlMsg asks Update to arm readControlCmd (pull one control msg).
type readControlMsg struct{}

// streamDoneMsg signals the ctrl channel was closed (all control drained).
type streamDoneMsg struct{}
