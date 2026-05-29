// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File loop.go — diagnose.Loop multi-turn orchestrator (spec-1.21 D-4).
//
// Loop is provider-agnostic and UI-agnostic. It drives a sequence of
// provider.Stream calls, dispatching tool_use to a ToolExecutor Registry
// between turns, and emits Events so the consumer (llmapp in production,
// tests in unit-mode) can render or assert. The package boundary is
// strict: this file MUST NOT import any UI / block / render package.
//
// Termination contract (spec-1.21 D-5):
//   - FinishStop / FinishStopSequence → natural; Run returns nil.
//   - FinishLength → natural truncation; Run returns nil.
//   - FinishRefusal → terminal with llm.ErrProviderRefusal.
//   - FinishCancelled → caller ctx cancel; Run returns ctx.Err.
//   - FinishError → terminal with the upstream error (provider, SDK,
//     per-turn LLM.TIMEOUT, total DIAGNOSE.TOTAL_TIMEOUT).
//   - FinishPause → DIAGNOSE.UNEXPECTED_PAUSE (spec-1.21 has no server-
//     side tool wired; defensive).
//   - FinishToolUse → recurse: execute, append paired turns, continue.
//   - Unknown tool name → DIAGNOSE.TOOL_UNKNOWN (terminal; assistant
//     turn NOT committed → 防 orphan tool_use → provider 400).
//   - maxTurns exceeded → DIAGNOSE.MAX_TURNS.
//
// Tool-result paired-commit (T-2 user 边界): the assistant turn carrying
// tool_use blocks AND the user turn carrying the matching tool_result
// blocks are appended to the message accumulator ATOMICALLY — either
// both or neither. A cancel / total timeout during tool execution
// rolls back both (well, never commits them).

package diagnose

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/sqlrush/opendbx/internal/domain/llm"
)

// Default Loop timeouts / bounds. Override via Options.
const (
	DefaultMaxTurns     = 16
	DefaultToolTimeout  = 30 * time.Second
	DefaultTotalTimeout = 10 * time.Minute
	DefaultReqTimeout   = 60 * time.Second
)

// Options is the Loop constructor input.
type Options struct {
	Provider llm.Provider // required
	Registry *Registry    // nil → no tools (only finish/length/error/refusal/etc. terminate)

	// MaxTurns caps the number of LLM round-trips. 0 → DefaultMaxTurns.
	MaxTurns int

	// ToolTimeout bounds a single ToolExecutor.Execute call. 0 → DefaultToolTimeout.
	ToolTimeout time.Duration

	// TotalTimeout bounds the whole Run. 0 → DefaultTotalTimeout.
	TotalTimeout time.Duration

	// ReqTimeout bounds one provider.Stream turn. 0 → DefaultReqTimeout.
	ReqTimeout time.Duration
}

// Loop is the constructed orchestrator. Safe for sequential Run calls;
// concurrent Run on the same Loop is not supported (each Run accumulates
// independent state).
type Loop struct {
	provider     llm.Provider
	registry     *Registry
	maxTurns     int
	toolTimeout  time.Duration
	totalTimeout time.Duration
	reqTimeout   time.Duration
}

// NewLoop constructs a Loop, applying defaults to zero-valued Options.
// A nil Provider is rejected as LLM.REQUEST_INVALID.
func NewLoop(opt Options) (*Loop, error) {
	if opt.Provider == nil {
		// errcode-lint:exempt -- spec-1.21 D-4: RequestInvalidf returns LLM.REQUEST_INVALID; pass-through.
		return nil, llm.RequestInvalidf("Loop requires non-nil Provider")
	}
	l := &Loop{
		provider:     opt.Provider,
		registry:     opt.Registry,
		maxTurns:     opt.MaxTurns,
		toolTimeout:  opt.ToolTimeout,
		totalTimeout: opt.TotalTimeout,
		reqTimeout:   opt.ReqTimeout,
	}
	if l.maxTurns <= 0 {
		l.maxTurns = DefaultMaxTurns
	}
	if l.toolTimeout <= 0 {
		l.toolTimeout = DefaultToolTimeout
	}
	if l.totalTimeout <= 0 {
		l.totalTimeout = DefaultTotalTimeout
	}
	if l.reqTimeout <= 0 {
		l.reqTimeout = DefaultReqTimeout
	}
	return l, nil
}

// Run executes the multi-turn loop. The returned Result is always non-
// zero-Messages (it carries the committed transcript) and FinishReason /
// TermCode describe the terminal cause. The returned error is non-nil
// for non-natural terminations; FinishStop / FinishStopSequence / FinishLength
// yield (Result, nil).
//
// Concurrency: Run consumes the provider stream synchronously and calls
// emit from this same goroutine. The consumer is expected to wrap emit
// in a select-on-ctx pattern so a UI cancel surfaces as emit returning
// ctx.Err.
//
//nolint:gocognit // spec-1.21 D-4: explicit 9-arm exhaustive FinishReason switch + paired-commit logic — splitting harms readability.
func (l *Loop) Run(ctx context.Context, req llm.Request, emit EmitFunc) (Result, error) {
	totalCtx, cancelTotal := context.WithTimeout(ctx, l.totalTimeout)
	defer cancelTotal()

	// Accumulator (append-only). req.Messages snapshot is the initial state.
	msgs := append([]llm.Message{}, req.Messages...)

	// Tool schemas: caller-supplied + registry. Caller takes precedence by
	// order (we don't dedupe — schema name collision is caller error and
	// will surface as a provider 400, which is the right blast radius).
	tools := append([]llm.ToolSchema{}, req.Tools...)
	if l.registry != nil {
		tools = append(tools, l.registry.Schemas()...)
	}

	result := Result{}

	for turn := 1; turn <= l.maxTurns; turn++ {
		result.Turns = turn

		if err := emit(ctx, Event{Kind: EventTurnStart, Turn: turn}); err != nil {
			fr, ferr := classifyEmitErr(err)
			// errcode-lint:exempt -- spec-1.21 D-4: ferr is the emit-side ctx/IO error returned by the consumer; Loop is a transparent middleman that surfaces it to the caller for classification.
			return finalize(result, msgs, fr, "", ferr), ferr
		}

		// Per-turn provider call.
		turnReq := req
		turnReq.Messages = msgs
		turnReq.Tools = tools
		turnCtx, cancelTurn := context.WithTimeout(totalCtx, l.reqTimeout)
		stream, perr := l.provider.Stream(turnCtx, turnReq)
		if perr != nil {
			cancelTurn()
			fr, code, ferr := classifyTerminal(perr, totalCtx)
			_ = emit(ctx, Event{Kind: EventFinish, Turn: turn, Finish: fr, TermCode: code, Err: ferr})
			// errcode-lint:exempt -- spec-1.21 D-4: ferr is either a registered LLM.* / DIAGNOSE.* sentinel (classifyTerminal) or the provider's already-classified errcode (anthropic/openai adapter pre-classify); pass-through.
			return finalize(result, msgs, fr, code, ferr), ferr
		}

		// Drain the stream, accumulating text / tool_uses / finish.
		var textBuf strings.Builder
		var toolUses []llm.ToolUse
		finish := llm.FinishUnset
		var finishErr error
		var emitErr error
		for stream.Next() {
			c := stream.Chunk()
			if c.Token != "" {
				if eerr := emit(ctx, Event{Kind: EventText, Turn: turn, Text: c.Token, Thinking: c.Thinking}); eerr != nil {
					emitErr = eerr
					break
				}
				if !c.Thinking {
					textBuf.WriteString(c.Token)
				}
			}
			if c.FinishReason != llm.FinishUnset {
				finish = c.FinishReason
				finishErr = c.Err
				if len(c.ToolUses) > 0 {
					toolUses = c.ToolUses
				}
			}
		}
		streamErr := stream.Err()
		_ = stream.Close()
		cancelTurn()

		if emitErr != nil {
			fr, ferr := classifyEmitErr(emitErr)
			// errcode-lint:exempt -- spec-1.21 D-4: emit-error pass-through; same contract as the EventTurnStart case above.
			return finalize(result, msgs, fr, "", ferr), ferr
		}

		// If the stream ended without an explicit finish, classify the
		// terminal cause from stream.Err() (most often ctx-related).
		if finish == llm.FinishUnset {
			if streamErr != nil {
				finish, _, finishErr = classifyTerminal(streamErr, totalCtx)
			} else {
				// Protocol anomaly — stream EOF without finish AND without err.
				finish, finishErr = llm.FinishError, llm.ErrStreamEmpty
			}
		}

		// Exhaustive switch on llm.FinishReason — no `default` that
		// silently absorbs a new enum (spec-1.21 D-4 user 边界:
		// 不留 default 吞掉新枚举; 新值入 enum 必须显式加 case).
		switch finish {
		case llm.FinishStop, llm.FinishStopSequence:
			_ = emit(ctx, Event{Kind: EventFinish, Turn: turn, Finish: finish})
			return finalize(result, msgs, finish, "", nil), nil

		case llm.FinishLength:
			_ = emit(ctx, Event{Kind: EventFinish, Turn: turn, Finish: finish})
			return finalize(result, msgs, finish, "", nil), nil

		case llm.FinishRefusal:
			_ = emit(ctx, Event{Kind: EventFinish, Turn: turn, Finish: finish, Err: llm.ErrProviderRefusal})
			return finalize(result, msgs, finish, "", llm.ErrProviderRefusal), llm.ErrProviderRefusal

		case llm.FinishCancelled:
			_ = emit(ctx, Event{Kind: EventFinish, Turn: turn, Finish: finish, Err: finishErr})
			// errcode-lint:exempt -- spec-1.21 D-4: finishErr is ctx-shaped (context.Canceled) from classifyTerminal; pass-through preserves Loop's transparency on user-initiated cancel.
			return finalize(result, msgs, finish, "", finishErr), finishErr

		case llm.FinishError:
			code := ""
			if errors.Is(finishErr, ErrTotalTimeout) {
				code = ErrTotalTimeout.Code()
			}
			_ = emit(ctx, Event{Kind: EventFinish, Turn: turn, Finish: finish, TermCode: code, Err: finishErr})
			// errcode-lint:exempt -- spec-1.21 D-4: finishErr is either a registered DIAGNOSE.* (TOTAL_TIMEOUT) / LLM.* (TIMEOUT / classified by provider adapter) sentinel, or the chunk-carried provider err that was already classified upstream.
			return finalize(result, msgs, finish, code, finishErr), finishErr

		case llm.FinishPause:
			_ = emit(ctx, Event{Kind: EventFinish, Turn: turn, Finish: finish, TermCode: ErrUnexpectedPause.Code(), Err: ErrUnexpectedPause})
			return finalize(result, msgs, finish, ErrUnexpectedPause.Code(), ErrUnexpectedPause), ErrUnexpectedPause

		case llm.FinishToolUse:
			// T-10a claude MED-1: defensive guard against a provider
			// protocol anomaly — FinishToolUse with no accumulated
			// tool_use blocks. The OpenAI adapter rejects this at the
			// stream level (stream.go FinishToolUse → ErrDecodeFailed
			// when len(tools)==0), but the Anthropic adapter's
			// decodeTools returns (nil, nil) on an empty toolAcc rather
			// than an error, so the Loop must defend itself: committing
			// an assistant turn with zero BlockToolUse blocks AND a
			// user turn with zero Content blocks would 400 the very
			// next Stream call. Treat as FinishError → ErrDecodeFailed.
			if len(toolUses) == 0 {
				_ = emit(ctx, Event{Kind: EventFinish, Turn: turn, Finish: llm.FinishError, Err: llm.ErrDecodeFailed})
				// errcode-lint:exempt -- spec-1.21 D-4: ErrDecodeFailed is a registered LLM.* sentinel (claude T-10a MED-1 absorb).
				return finalize(result, msgs, llm.FinishError, "", llm.ErrDecodeFailed), llm.ErrDecodeFailed
			}
			// Validate ALL tool names against the registry — any
			// unknown name → TOOL_UNKNOWN terminal without committing
			// the assistant turn (spec-1.21 D-4 step 4 / T-2 HIGH-5
			// 防 orphan tool_use → provider 400 on resume).
			for i := range toolUses {
				if _, ok := lookup(l.registry, toolUses[i].Name); !ok {
					_ = emit(ctx, Event{Kind: EventFinish, Turn: turn, Finish: finish, TermCode: ErrToolUnknown.Code(), Err: ErrToolUnknown})
					return finalize(result, msgs, finish, ErrToolUnknown.Code(), ErrToolUnknown), ErrToolUnknown
				}
			}

			// Build the assistant turn payload (text accumulated this
			// turn + all tool_use blocks). NOT committed yet — paired
			// commit happens after every tool finishes.
			asstContent := make([]llm.ContentBlock, 0, len(toolUses)+1)
			if textBuf.Len() > 0 {
				asstContent = append(asstContent, llm.ContentBlock{Type: llm.BlockText, Text: textBuf.String()})
			}
			for i := range toolUses {
				tu := toolUses[i]
				asstContent = append(asstContent, llm.NewToolUseBlock(&tu))
			}

			// Execute each tool. Collect ToolResults; terminate eagerly
			// on cancel / total timeout (paired commit is skipped → no
			// dangling tool_use in the transcript).
			results := make([]llm.ToolResult, 0, len(toolUses))
			for i := range toolUses {
				tu := &toolUses[i]
				if eerr := emit(ctx, Event{Kind: EventToolCall, Turn: turn, ToolUse: tu}); eerr != nil {
					fr, ferr := classifyEmitErr(eerr)
					// errcode-lint:exempt -- spec-1.21 D-4: emit-error pass-through (EventToolCall variant).
					return finalize(result, msgs, fr, "", ferr), ferr
				}
				exec, _ := lookup(l.registry, tu.Name)
				toolCtx, cancelTool := context.WithTimeout(totalCtx, l.toolTimeout)
				out, execErr := exec.Execute(toolCtx, tu.Input)
				cancelTool()

				tr, term, fr, code, ferr := classifyToolErr(tu.ID, out, execErr, totalCtx)
				if term {
					_ = emit(ctx, Event{Kind: EventFinish, Turn: turn, Finish: fr, TermCode: code, Err: ferr})
					// errcode-lint:exempt -- spec-1.21 D-4: ferr is a registered DIAGNOSE.TOTAL_TIMEOUT sentinel or ctx.Canceled from classifyToolErr; pass-through.
					return finalize(result, msgs, fr, code, ferr), ferr
				}
				results = append(results, tr)
				if eerr := emit(ctx, Event{Kind: EventToolResult, Turn: turn, ToolResult: &results[len(results)-1]}); eerr != nil {
					fr, ferr := classifyEmitErr(eerr)
					// errcode-lint:exempt -- spec-1.21 D-4: emit-error pass-through (EventToolResult variant).
					return finalize(result, msgs, fr, "", ferr), ferr
				}
			}

			// Atomic paired commit: assistant turn + user turn together.
			msgs = append(msgs, llm.Message{Role: llm.RoleAssistant, Content: asstContent})
			userBlocks := make([]llm.ContentBlock, 0, len(results))
			for i := range results {
				userBlocks = append(userBlocks, llm.NewToolResultBlock(&results[i]))
			}
			msgs = append(msgs, llm.Message{Role: llm.RoleUser, Content: userBlocks})
			// next turn

		case llm.FinishUnset:
			// Should be unreachable — we promoted FinishUnset above.
			_ = emit(ctx, Event{Kind: EventFinish, Turn: turn, Finish: llm.FinishError, Err: llm.ErrDecodeFailed})
			return finalize(result, msgs, llm.FinishError, "", llm.ErrDecodeFailed), llm.ErrDecodeFailed
		}
		// Note: no `default` clause — Go's exhaustiveness is enforced
		// by review (规则 21 共用组件改动 + spec-1.21 D-4 user 边界).
		// A new FinishReason added without a case here is a compile-
		// time visible diff and the spec lint catches the spec drift.
	}

	// maxTurns exhausted without natural finish.
	_ = emit(ctx, Event{Kind: EventFinish, Turn: result.Turns, Finish: llm.FinishError, TermCode: ErrMaxTurns.Code(), Err: ErrMaxTurns})
	return finalize(result, msgs, llm.FinishError, ErrMaxTurns.Code(), ErrMaxTurns), ErrMaxTurns
}

// finalize populates the terminal Result fields and returns by value.
func finalize(r Result, msgs []llm.Message, fr llm.FinishReason, code string, _ error) Result {
	r.Messages = msgs
	r.FinishReason = fr
	r.TermCode = code
	return r
}

// classifyTerminal maps a stream / provider construction error to a
// (FinishReason, TermCode, error) tuple. The three buckets are:
//   - ctx.Canceled — propagated from caller ctx → FinishCancelled.
//   - ctx.DeadlineExceeded — total deadline fired → DIAGNOSE.TOTAL_TIMEOUT;
//     otherwise per-turn deadline fired → LLM.TIMEOUT.
//   - anything else — bare FinishError carrying the upstream error.
//
// Distinguishing "total fired" vs "per-turn fired" relies on inspecting
// totalCtx.Err() after the deadline: only the total ctx reports
// DeadlineExceeded when total fired; a per-turn-only timeout leaves
// totalCtx.Err() nil.
func classifyTerminal(streamErr error, totalCtx context.Context) (llm.FinishReason, string, error) {
	if streamErr == nil {
		return llm.FinishUnset, "", nil
	}
	switch {
	case errors.Is(streamErr, context.Canceled):
		if errors.Is(totalCtx.Err(), context.DeadlineExceeded) {
			return llm.FinishError, ErrTotalTimeout.Code(), ErrTotalTimeout
		}
		return llm.FinishCancelled, "", streamErr
	case errors.Is(streamErr, context.DeadlineExceeded):
		if errors.Is(totalCtx.Err(), context.DeadlineExceeded) {
			return llm.FinishError, ErrTotalTimeout.Code(), ErrTotalTimeout
		}
		return llm.FinishError, "", llm.ErrTimeout
	default:
		return llm.FinishError, "", streamErr
	}
}

// classifyToolErr maps a single tool's Execute outcome to the terminal-
// vs-feedback contract (spec-1.21 D-5).
//
// Returned tuple semantics:
//   - tr        — when terminate=false, the ToolResult to append to the
//     user turn (Content/IsError set per the success/timeout/error path).
//   - terminate — true when the loop must terminate; the (fr, code, err)
//     fields then describe the cause.
func classifyToolErr(toolUseID string, out ToolOutput, execErr error, totalCtx context.Context) (
	tr llm.ToolResult, terminate bool, fr llm.FinishReason, code string, ferr error,
) {
	switch {
	case errors.Is(execErr, context.Canceled):
		return llm.ToolResult{}, true, llm.FinishCancelled, "", execErr
	case errors.Is(execErr, context.DeadlineExceeded):
		if errors.Is(totalCtx.Err(), context.DeadlineExceeded) {
			return llm.ToolResult{}, true, llm.FinishError, ErrTotalTimeout.Code(), ErrTotalTimeout
		}
		// Per-tool deadline → feedback (LLM self-correct), NOT terminal.
		return llm.ToolResult{ToolUseID: toolUseID, Content: ErrToolTimeout.Error(), IsError: true}, false, 0, "", nil
	case execErr != nil:
		return llm.ToolResult{ToolUseID: toolUseID, Content: execErr.Error(), IsError: true}, false, 0, "", nil
	default:
		return llm.ToolResult{ToolUseID: toolUseID, Content: out.Content, IsError: out.IsError}, false, 0, "", nil
	}
}

// classifyEmitErr maps an emit-side error to a terminal disposition.
// ctx-shaped errors → FinishCancelled (consumer requested stop). Other
// errors (consumer-side IO failures) → FinishError.
func classifyEmitErr(emitErr error) (llm.FinishReason, error) {
	if errors.Is(emitErr, context.Canceled) || errors.Is(emitErr, context.DeadlineExceeded) {
		return llm.FinishCancelled, emitErr
	}
	return llm.FinishError, emitErr
}

// lookup is a nil-safe Registry.Get wrapper used by the dispatch path so
// a nil Registry behaves identically to an empty one (no tool found).
func lookup(r *Registry, name string) (ToolExecutor, bool) {
	if r == nil {
		return nil, false
	}
	return r.Get(name)
}
