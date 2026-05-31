// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File loop_adapter.go — bridges diagnose.Loop events into the existing
// llmapp control plumbing (streamControlMsg + readControlMsg + ctrl
// channel), preserving the spec-1.20 R2 CRIT-2 token-vs-control split:
//
//   - EventText visible  → TokenStream.AppendChunk + streamControlMsg
//   - EventText thinking → streamControlMsg{ThinkingToken}, never TokenStream
//   - EventToolCall  → streamControlMsg carrying *llm.ToolUse
//   - EventToolResult → streamControlMsg carrying *llm.ToolResult
//   - EventFinish    → streamControlMsg carrying Finish + TermCode + Err
//   - EventTurnStart → no-op on the UI (turn boundaries are not yet
//     rendered; reserved for future progress indicators)
//
// Backpressure parity (spec-1.21 T-2.1 HIGH-2 / D-6): every ctrl send
// is wrapped in a select on the emit-call ctx so a user cancel does not
// deadlock Loop's goroutine on a full ctrl chan.

package llmapp

import (
	"context"

	"github.com/sqlrush/opendbx/internal/app/cli/render/scheduler"
	"github.com/sqlrush/opendbx/internal/app/cli/render/streaming"
	"github.com/sqlrush/opendbx/internal/app/diagnose"
	"github.com/sqlrush/opendbx/internal/domain/llm"
)

// makeEmit returns a diagnose.EmitFunc routing Loop events to the
// llmapp plumbing. ts and ctrl are owned by submit() and closed by the
// loopStartCmd goroutine after Run returns.
//
// stripThink suppresses thinking-channel token payloads from the control
// message. The Thinking flag still surfaces so the Model can distinguish a
// thinking-only response from a genuinely empty stream.
func makeEmit(ts *streaming.TokenStream, ctrl chan<- streamControlMsg, stripThink bool) diagnose.EmitFunc {
	send := func(ctx context.Context, msg streamControlMsg) error {
		select {
		case ctrl <- msg:
			return nil
		case <-ctx.Done():
			// errcode-lint:exempt -- spec-1.21 D-6: ctx.Err pass-through; Loop classifies cancel-vs-timeout (classifyEmitErr).
			return ctx.Err()
		}
	}
	return func(ctx context.Context, e diagnose.Event) error {
		switch e.Kind {
		case diagnose.EventText:
			if e.Thinking {
				msg := streamControlMsg{Thinking: true}
				if !stripThink {
					msg.ThinkingToken = e.Text
				}
				return send(ctx, msg)
			}
			visible := e.Text != ""
			if visible {
				_ = ts.AppendChunk(streaming.Chunk{Token: e.Text})
			}
			return send(ctx, streamControlMsg{VisibleContent: visible})
		case diagnose.EventToolCall:
			return send(ctx, streamControlMsg{ToolUse: e.ToolUse})
		case diagnose.EventToolResult:
			return send(ctx, streamControlMsg{ToolResult: e.ToolResult, Cached: e.Cached})
		case diagnose.EventFinish:
			// Push a terminal chunk so the TokenStream's per-finish
			// branches (Length truncation marker / cancel state) keep
			// working under the new emit path (spec-1.6 R2.2 / spec-1.20
			// thinking-only Empty placeholder).
			_ = ts.AppendChunk(streaming.Chunk{
				FinishReason: mapToRender(e.Finish),
				Err:          e.Err,
			})
			return send(ctx, streamControlMsg{
				Finish:   e.Finish,
				TermCode: e.TermCode,
				Err:      e.Err,
			})
		case diagnose.EventTurnStart:
			// Turn boundaries are not surfaced to the UI in this stage;
			// a future spec may add a per-turn marker / progress line.
			return nil
		}
		return nil
	}
}

// loopStartCmd launches a diagnose.Loop.Run on the worker pool. The
// goroutine drains the loop to terminal, then closes ctrl and ts so the
// reader-Cmd observes a done signal (mirrors the spec-1.20 consumeStream
// teardown order — close ts BEFORE ctrl is the convention so the
// streamDoneMsg drain sees a fully-flushed TokenStream).
func loopStartCmd(
	ctx context.Context,
	loop *diagnose.Loop,
	req llm.Request,
	ts *streaming.TokenStream,
	ctrl chan streamControlMsg,
	stripThink bool,
) scheduler.Cmd {
	return func() scheduler.Msg {
		go func() {
			defer close(ctrl)
			defer func() { _ = ts.Close() }()
			emit := makeEmit(ts, ctrl, stripThink)
			// Result / err are surfaced via EventFinish through ctrl; we
			// intentionally drop them here. The Loop is the single source
			// of truth for classification (spec-1.21 D-4) — duplicating
			// it on the goroutine side would invite drift.
			_, _ = loop.Run(ctx, req, emit)
		}()
		return readControlMsg{}
	}
}
