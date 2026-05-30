// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package llmapp

import (
	"github.com/sqlrush/opendbx/internal/app/cli/render/streaming"
	"github.com/sqlrush/opendbx/internal/domain/llm"
)

// streamControlMsg is the control-bypass Msg (spec-1.20 R2 CRIT-2 base;
// spec-1.21 D-6 extension). Routing is by which optional field is set:
//
//   - ToolUse != nil       → EventToolCall variant
//   - ToolResult != nil    → EventToolResult variant
//   - Finish.Terminal()    → EventFinish variant (TermCode optional DIAGNOSE.*)
//   - Thinking=true       → EventText thinking-channel variant; ThinkingToken
//     carries renderable thinking only when strip_think=false. Thinking never
//     enters the TokenStream / main content plane.
//   - default (all nil)    → EventText visible-text variant; VisibleContent
//     tells Update to accumulate sawContent without inspecting TokenStream
//     (otherwise text + FinishLength would be misjudged !sawContent → false
//     STREAM_EMPTY).
//
// At most one variant is populated per message; mixed shapes are not
// produced by makeEmit. Update dispatches on the variants in priority
// (ToolUse / ToolResult before Finish before text) so a future spec
// adding fields keeps the routing explicit.
type streamControlMsg struct {
	VisibleContent bool
	Thinking       bool
	ThinkingToken  string          // EventText thinking side channel (spec-1.20.2 D-5)
	ToolUse        *llm.ToolUse    // EventToolCall (spec-1.21 D-6)
	ToolResult     *llm.ToolResult // EventToolResult (spec-1.21 D-6)
	Finish         llm.FinishReason
	TermCode       string // EventFinish — DIAGNOSE.* code or "" on natural Stop
	Err            error
}

// readControlMsg asks Update to arm readControlCmd (pull one control msg).
type readControlMsg struct{}

// streamDoneMsg signals the ctrl channel was closed (all control drained).
type streamDoneMsg struct{}

// mapToRender maps an llm.FinishReason to the renderable streaming subset
// (spec-1.20 R2 H-2). Non-renderable terminal reasons (ToolUse / Pause /
// Refusal / StopSequence) map to streaming.FinishStop; the precise reason
// travels via the control channel. T1 exhaustive test covers all 9.
func mapToRender(f llm.FinishReason) streaming.FinishReason {
	switch f {
	case llm.FinishUnset:
		return streaming.FinishUnset
	case llm.FinishLength:
		return streaming.FinishLength
	case llm.FinishError:
		return streaming.FinishError
	case llm.FinishCancelled:
		return streaming.FinishCancelled
	default:
		// Stop / ToolUse / StopSequence / Pause / Refusal → clean stop for
		// the render stream; control channel carries the precise reason.
		return streaming.FinishStop
	}
}
