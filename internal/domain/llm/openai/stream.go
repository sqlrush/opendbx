// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package openai

import (
	"bytes"
	"encoding/json"
	"errors"
	"sort"
	"sync"

	openaisdk "github.com/openai/openai-go"
	"github.com/openai/openai-go/packages/ssestream"

	"github.com/sqlrush/opendbx/internal/domain/llm"
)

// stream wraps the SDK SSE stream as an llm.Stream iterator (spec-1.20.1
// D-1/D-3). Content delta → Token; reasoning_content (vendor raw, B-64 无
// typed 字段) → Thinking; tool_calls 累积 (capture-on-first + MaxToolBlocks +
// pre-write bound + sort-by-index + shared DecodeToolInput); finish 全枚举
// (function_call→ToolUse, content_filter→Error+CONTENT_FILTERED, unknown→Error).
type stream struct {
	sdk       *ssestream.Stream[openaisdk.ChatCompletionChunk]
	cur       llm.Chunk
	err       error
	done      bool
	toolAcc   map[int64]*toolBlock // tool_call index → accumulating tool call
	closeOnce sync.Once            // SDK ssestream.Stream.Close 非幂等 (同 anthropic)
}

type toolBlock struct {
	id   string
	name string
	buf  bytes.Buffer // function.arguments partial JSON accumulation
}

func newStream(sdk *ssestream.Stream[openaisdk.ChatCompletionChunk]) *stream {
	return &stream{sdk: sdk, toolAcc: map[int64]*toolBlock{}}
}

// Next advances to the next emittable llm.Chunk. Returns false at end/error.
func (s *stream) Next() bool {
	if s.done || s.err != nil {
		return false
	}
	for s.sdk.Next() {
		chunk, emit := s.mapChunk(s.sdk.Current())
		// mapChunk may set s.err on a hard guard violation (oversize tool
		// input / too many tool blocks) — stop so s.Err() surfaces it.
		if s.err != nil {
			s.done = true
			return false
		}
		if emit {
			s.cur = chunk
			return true
		}
	}
	if err := s.sdk.Err(); err != nil {
		s.err = err
	}
	s.done = true
	return false
}

// Chunk returns the current chunk.
func (s *stream) Chunk() llm.Chunk { return s.cur }

// Err returns the terminal error, mapped to a registered LLM.* errcode
// (原则 3 + 规则 7: a raw SDK transport/API error must not escape un-classified).
func (s *stream) Err() error { return classifyOpenAIErr(s.err) }

// classifyOpenAIErr maps an SDK API error to a registered LLM.* errcode by
// HTTP status (mirror anthropic classifyStreamErr). Non-SDK errors (ctx
// cancel/deadline, our own ErrDecodeFailed) pass through unchanged — ctx is
// classified by the app-layer finishFromErr; ErrDecodeFailed is already a
// registered code.
func classifyOpenAIErr(err error) error {
	if err == nil {
		return nil
	}
	var apiErr *openaisdk.Error
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case 400, 422:
			return llm.ErrRequestInvalid
		case 401, 403:
			return llm.ErrAuthFailed
		case 408:
			return llm.ErrTimeout
		default:
			// 404 model/endpoint not found / 429 rate-limit / 5xx / unknown.
			return llm.ErrUnavailable
		}
	}
	return err
}

// Close releases the SDK stream (idempotent via sync.Once — ssestream.Stream
// .Close is not itself idempotent, same hazard as anthropic).
func (s *stream) Close() error {
	var err error
	s.closeOnce.Do(func() { err = s.sdk.Close() })
	return err
}

// mapChunk maps one SDK chunk to (llm.Chunk, emit?). Routing: reasoning_content
// (raw) → Thinking; Content → Token; tool_calls → accumulate; finish_reason →
// mapFinish. May set s.err on a tool-accumulation guard violation.
func (s *stream) mapChunk(c openaisdk.ChatCompletionChunk) (llm.Chunk, bool) {
	if len(c.Choices) == 0 {
		return llm.Chunk{}, false
	}
	choice := c.Choices[0]
	delta := choice.Delta
	if rc := reasoningContent(delta); rc != "" {
		return llm.Chunk{Token: rc, Thinking: true}, true
	}
	if delta.Content != "" {
		return llm.Chunk{Token: delta.Content}, true
	}
	for i := range delta.ToolCalls {
		if err := s.accumulateToolCall(delta.ToolCalls[i]); err != nil {
			s.err = err
			return llm.Chunk{}, false
		}
	}
	if choice.FinishReason != "" {
		return s.mapFinish(choice.FinishReason)
	}
	return llm.Chunk{}, false
}

// accumulateToolCall accumulates a streamed tool_call delta by index. id/name
// arrive only in the first delta for an index (capture-on-first; later deltas
// have empty id/name). Bounds concurrent blocks (MaxToolBlocks) and per-block
// arg size BEFORE the write (mirror anthropic T-10a HIGH-2 DoS defense).
func (s *stream) accumulateToolCall(tc openaisdk.ChatCompletionChunkChoiceDeltaToolCall) error {
	tb := s.toolAcc[tc.Index]
	if tb == nil {
		if len(s.toolAcc) >= llm.MaxToolBlocks {
			return llm.ErrDecodeFailed
		}
		tb = &toolBlock{}
		s.toolAcc[tc.Index] = tb
	}
	if tc.ID != "" {
		tb.id = tc.ID
	}
	if tc.Function.Name != "" {
		tb.name = tc.Function.Name
	}
	args := tc.Function.Arguments
	if tb.buf.Len()+len(args) > llm.MaxToolInputBytes {
		return llm.ErrDecodeFailed
	}
	tb.buf.WriteString(args)
	return nil
}

// mapFinish maps OpenAI finish_reason → terminal llm.Chunk. function_call is
// the legacy Functions API path (structurally tool_calls). content_filter →
// FinishError + LLM.CONTENT_FILTERED (不污染 FROZEN FinishRefusal). unknown →
// FinishError (原则 3 显式失败, 非静默 Stop; codex HIGH-4).
func (s *stream) mapFinish(fr string) (llm.Chunk, bool) {
	switch fr {
	case "stop":
		return llm.Chunk{FinishReason: llm.FinishStop}, true
	case "length":
		return llm.Chunk{FinishReason: llm.FinishLength}, true
	case "tool_calls", "function_call":
		tools, err := s.decodeTools()
		if err != nil {
			return llm.Chunk{FinishReason: llm.FinishError, Err: err}, true
		}
		return llm.Chunk{FinishReason: llm.FinishToolUse, ToolUses: tools}, true
	case "content_filter":
		return llm.Chunk{FinishReason: llm.FinishError, Err: llm.ErrContentFiltered}, true
	default:
		return llm.Chunk{FinishReason: llm.FinishError, Err: llm.ErrDecodeFailed}, true
	}
}

// decodeTools decodes accumulated tool calls in ascending index order
// (deterministic for spec-1.21's executor) with the shared JSON guard.
func (s *stream) decodeTools() ([]llm.ToolUse, error) {
	if len(s.toolAcc) == 0 {
		return nil, nil
	}
	keys := make([]int64, 0, len(s.toolAcc))
	for k := range s.toolAcc {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	out := make([]llm.ToolUse, 0, len(keys))
	for _, k := range keys {
		tb := s.toolAcc[k]
		input, err := llm.DecodeToolInput(tb.buf.Bytes())
		if err != nil {
			return nil, err
		}
		out = append(out, llm.ToolUse{ID: tb.id, Name: tb.name, Input: input})
	}
	return out, nil
}

// reasoningContent reads vendor reasoning_content from the delta's raw extra
// fields (B-64: openai-go has no typed ReasoningContent field; deepseek-reasoner
// / o1 风格). Empty if absent / null / non-string.
func reasoningContent(delta openaisdk.ChatCompletionChunkChoiceDelta) string {
	f, ok := delta.JSON.ExtraFields["reasoning_content"]
	if !ok {
		return ""
	}
	raw := f.Raw()
	if raw == "" || raw == "null" {
		return ""
	}
	var rc string
	if json.Unmarshal([]byte(raw), &rc) != nil {
		return ""
	}
	return rc
}
