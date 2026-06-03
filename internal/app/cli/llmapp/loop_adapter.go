// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File loop_adapter.go — bridges diagnose.Loop events into llmapp's
// control plumbing (streamControlMsg + readControlMsg + ctrl channel).
// spec-1.21.1 fixes the old token-vs-control merge race by making text
// authoritative through the same ctrl FIFO as tool events:
//
//   - EventText visible  → segBuf + TokenStream preview + PreviewTick
//   - EventText thinking → streamControlMsg{ThinkingToken}, never TokenStream
//   - EventToolCall     → seal visible segBuf, then *llm.ToolUse
//   - EventToolResult   → seal visible segBuf, then *llm.ToolResult
//   - EventFinish       → seal visible segBuf, then Finish + TermCode + Err
//   - EventTurnStart    → no-op on the UI (turn boundaries are not yet
//     rendered; reserved for future progress indicators)
//
// Backpressure parity (spec-1.21 T-2.1 HIGH-2 / D-6): every ctrl send
// is wrapped in a select on the emit-call ctx so a user cancel does not
// deadlock Loop's goroutine on a full ctrl chan.

package llmapp

import (
	"context"
	"strings"
	"time"

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
func makeEmit(ts *streaming.TokenStream, ctrl chan<- streamControlMsg, stripThink bool, sb *snapshotBuilder) diagnose.EmitFunc {
	send := func(ctx context.Context, msg streamControlMsg) error {
		select {
		case ctrl <- msg:
			return nil
		case <-ctx.Done():
			// errcode-lint:exempt -- spec-1.21 D-6: ctx.Err pass-through; Loop classifies cancel-vs-timeout (classifyEmitErr).
			return ctx.Err()
		}
	}

	// spec-1.21.1 segment-mode contract: makeEmit is called by the
	// diagnose.Loop producer goroutine in event order, so segBuf is
	// intentionally goroutine-local and unsynchronized. Do not reuse this
	// closure from multiple producers without re-specifying ordering.
	var segBuf strings.Builder // spec-1.21.1 R-fix MED-1: O(n) accumulation (was string concat)
	seal := func(ctx context.Context, truncated bool) error {
		if segBuf.Len() == 0 {
			return nil
		}
		text := segBuf.String()
		segBuf.Reset()
		return send(ctx, streamControlMsg{SealedText: text, SealedTruncated: truncated})
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
			if e.Text == "" {
				return nil
			}
			segBuf.WriteString(e.Text)
			_ = ts.AppendChunk(streaming.Chunk{Token: e.Text})
			sb.addText(e.Text) // spec-1.23 D-3: accumulate the visible final answer
			return send(ctx, streamControlMsg{PreviewTick: true})
		case diagnose.EventToolCall:
			if err := seal(ctx, false); err != nil {
				return err
			}
			sb.addToolCall(e.ToolUse) // spec-1.23 D-3
			return send(ctx, streamControlMsg{ToolUse: e.ToolUse})
		case diagnose.EventToolResult:
			if err := seal(ctx, false); err != nil {
				return err
			}
			sb.addToolResult(e.ToolResult, e.Cached) // spec-1.23 D-3
			return send(ctx, streamControlMsg{ToolResult: e.ToolResult, Cached: e.Cached})
		case diagnose.EventFinish:
			if err := seal(ctx, e.Finish == llm.FinishLength); err != nil {
				return err
			}
			// spec-1.23 D-3: seal the run snapshot atomically with the finish
			// and ride it on this same control msg — no separate post-Run send,
			// so no channel-close race (three-route T-2 收敛 fix).
			return send(ctx, streamControlMsg{
				Finish:   e.Finish,
				TermCode: e.TermCode,
				Err:      e.Err,
				Snapshot: sb.seal(e),
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
// teardown order — close ts BEFORE ctrl is the convention). On the normal
// path authoritative text has already travelled through ctrl as SealedText, so
// streamDoneMsg only performs cleanup; on a cancel mid-segment streamDoneMsg
// recovers the residual partial text from previewNodes (spec-1.21.1 R-fix H-1).
func loopStartCmd(
	ctx context.Context,
	loop *diagnose.Loop,
	req llm.Request,
	userText string,
	ts *streaming.TokenStream,
	ctrl chan streamControlMsg,
	stripThink bool,
) scheduler.Cmd {
	return func() scheduler.Msg {
		go func() {
			defer close(ctrl)
			defer func() { _ = ts.Close() }()
			// spec-1.23 D-3: capture the run snapshot from the Event stream.
			// Goroutine-local; only the sealed value escapes via EventFinish.
			sb := newSnapshotBuilder(userText, time.Now)
			emit := makeEmit(ts, ctrl, stripThink, sb)
			// Result / err are surfaced via EventFinish through ctrl; we
			// intentionally drop them here. The Loop is the single source
			// of truth for classification (spec-1.21 D-4) — duplicating
			// it on the goroutine side would invite drift.
			_, _ = loop.Run(ctx, req, emit)
		}()
		return readControlMsg{}
	}
}
