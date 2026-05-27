// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package llmapp

import (
	"context"
	"errors"

	"github.com/sqlrush/opendbx/internal/app/cli/render/streaming"
	"github.com/sqlrush/opendbx/internal/domain/llm"
)

// streamControlMsg is the control-bypass Msg (spec-1.20 R2 CRIT-2: Token
// flows to the TokenStream; control flows here). VisibleContent (R2.2
// HIGH-2) marks whether this chunk carried visible text so Update can
// accumulate sawContent without inspecting the TokenStream — otherwise
// text + FinishLength would be misjudged !sawContent → false STREAM_EMPTY.
type streamControlMsg struct {
	VisibleContent bool
	Thinking       bool
	ToolUses       []llm.ToolUse
	Finish         llm.FinishReason
	Err            error
}

// readControlMsg asks Update to arm readControlCmd (pull one control msg).
type readControlMsg struct{}

// streamDoneMsg signals the ctrl channel was closed (all control drained).
type streamDoneMsg struct{}

// finishFromErr classifies a terminal error into (FinishReason, mapped
// err). spec-1.20 R2.2 MED: SINGLE classifier reused by both the
// streamStartCmd immediate-error path and consumeStream's s.Err() — so a
// Ctrl+C during connect (provider.Stream returns context.Canceled) maps
// to FinishCancelled, not a generic FinishError.
func finishFromErr(err error) (llm.FinishReason, error) {
	switch {
	case err == nil:
		return llm.FinishStop, nil
	case errors.Is(err, context.Canceled):
		return llm.FinishCancelled, nil
	case errors.Is(err, context.DeadlineExceeded):
		return llm.FinishError, llm.ErrTimeout
	default:
		return llm.FinishError, err
	}
}

// mapToRender maps an llm.FinishReason to the renderable streaming subset
// (spec-1.20 R2 H-2). Non-renderable terminal reasons (ToolUse / Pause /
// Refusal / StopSequence) map to streaming.FinishStop; the real reason
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

// consumeStream runs in the streamStartCmd goroutine (NOT Update). It
// iterates the llm.Stream, routing each chunk: renderable Token →
// TokenStream; control → ctrl chan. It closes ctrl when the stream ends
// so the reader-Cmd observes a done signal (spec-1.20 R2.2).
func consumeStream(s llm.Stream, ts *streaming.TokenStream, ctrl chan<- streamControlMsg, stripThink bool) {
	defer func() { _ = s.Close() }() // best-effort release; terminal Err already surfaced via ctrl
	for s.Next() {
		c := s.Chunk()
		visible := c.Token != "" && !c.Thinking
		if visible || (c.Thinking && !stripThink) {
			_ = ts.AppendChunk(streaming.Chunk{Token: c.Token, FinishReason: mapToRender(c.FinishReason), Err: c.Err})
		}
		ctrl <- streamControlMsg{
			VisibleContent: visible,
			Thinking:       c.Thinking,
			ToolUses:       c.ToolUses,
			Finish:         c.FinishReason,
			Err:            c.Err,
		}
	}
	if err := s.Err(); err != nil {
		fr, mapped := finishFromErr(err)
		ctrl <- streamControlMsg{Finish: fr, Err: mapped}
	}
	_ = ts.Close()
	close(ctrl)
}
