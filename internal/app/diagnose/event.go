// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File event.go — Event / Result / EmitFunc (spec-1.21 D-4).
//
// These types are the orchestration <-> consumer contract for diagnose.
// Loop. They are intentionally framework-agnostic: the diagnose package
// MUST NOT import any UI (llmapp / render / block) package — llmapp is
// the consumer that translates Events into spec-1.9 block.ToolUse /
// spec-1.9b block.ToolResult render nodes (spec-1.21 D-6).

package diagnose

import (
	"context"

	"github.com/sqlrush/opendbx/internal/domain/llm"
)

// EventKind identifies the payload variant carried by an Event.
type EventKind int

// Event kinds (append-only; renderers MUST handle the union exhaustively
// — a future kind added here is a forward-compatible additive change but
// consumers should surface unknown kinds rather than silently drop).
const (
	EventTurnStart  EventKind = iota // new turn began (Turn populated)
	EventText                        // streamed text token (Text + Thinking)
	EventToolCall                    // tool dispatch (ToolUse populated; emitted BEFORE Execute)
	EventToolResult                  // tool finished (ToolResult populated; emitted AFTER Execute)
	EventFinish                      // terminal — Finish + TermCode + Err describe the cause
)

// Event is the single value flowing from Loop.Run to its consumer via
// EmitFunc. Only the fields relevant to Kind are populated; the others
// are zero. ToolUse / ToolResult pointers reference Loop-owned storage
// that remains valid for the duration of the emit call.
type Event struct {
	Kind       EventKind
	Turn       int              // 1-based turn counter
	Text       string           // EventText
	Thinking   bool             // EventText — true → thinking-channel token
	ToolUse    *llm.ToolUse     // EventToolCall
	ToolResult *llm.ToolResult  // EventToolResult
	Cached     bool             // EventToolResult — spec-1.22: result served from dedup cache (render-only signal; the ToolResult content is byte-identical to a fresh run, CLAUDE.md § 3.6 errata)
	Finish     llm.FinishReason // EventFinish
	TermCode   string           // EventFinish — DIAGNOSE.* code ("" on natural FinishStop / FinishStopSequence)
	Err        error            // EventFinish — registered errcode (LLM.* / DIAGNOSE.*) or ctx.Err
}

// EmitFunc is the consumer-side sink. It MAY block while delivering;
// implementations SHOULD wrap the send in a select on the supplied ctx
// so a cancellation propagates as the returned error (Loop will then
// halt promptly — spec-1.21 T-2.1 HIGH-2 backpressure/cancel parity
// with the spec-1.20 consumeStream contract).
type EmitFunc func(context.Context, Event) error

// Result is the terminal summary returned by Loop.Run. Messages contains
// the full multi-turn transcript (req.Messages + every committed
// assistant/user pair); spec-1.23 fault reports and spec-2.10 memory
// consume it directly.
//
// FinishReason mirrors the final terminal cause as observed by the
// consumer's EventFinish. TermCode is "" on a natural FinishStop /
// FinishStopSequence; otherwise it is the DIAGNOSE.* code (e.g.
// "DIAGNOSE.TOTAL_TIMEOUT") so the renderer can pick the correct
// terminal node template without re-classifying the error.
type Result struct {
	Messages     []llm.Message
	FinishReason llm.FinishReason
	Turns        int
	TermCode     string
}
