// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package openai

import (
	"sync"

	openaisdk "github.com/openai/openai-go"
	"github.com/openai/openai-go/packages/ssestream"

	"github.com/sqlrush/opendbx/internal/domain/llm"
)

// stream wraps the SDK SSE stream as an llm.Stream iterator (spec-1.20.1
// D-1). T-4 骨架: Content delta → Token; finish_reason stop/length. 完整
// delta — reasoning_content (raw Delta.JSON.ExtraFields, B-64 无 typed 字段) /
// tool_calls 累积 (capture-on-first + MaxToolBlocks + pre-write bound +
// sort-by-index + DecodeToolInput) / finish 全枚举 (function_call→ToolUse,
// content_filter→Error+CONTENT_FILTERED, unknown→Error) → T-5.
type stream struct {
	sdk       *ssestream.Stream[openaisdk.ChatCompletionChunk]
	cur       llm.Chunk
	err       error
	done      bool
	closeOnce sync.Once // SDK ssestream.Stream.Close 非幂等 (同 anthropic)
}

func newStream(sdk *ssestream.Stream[openaisdk.ChatCompletionChunk]) *stream {
	return &stream{sdk: sdk}
}

// Next advances to the next emittable llm.Chunk. Returns false at end/error.
func (s *stream) Next() bool {
	if s.done || s.err != nil {
		return false
	}
	for s.sdk.Next() {
		chunk, emit := mapChunk(s.sdk.Current())
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

// Err returns the terminal error. T-6 maps via classifyOpenAIErr
// (openai.Error.StatusCode → LLM.* + ctx/ErrDecodeFailed pass-through);
// T-4 骨架 returns the raw SDK/transport error.
func (s *stream) Err() error { return s.err }

// Close releases the SDK stream (idempotent via sync.Once — ssestream.Stream
// .Close is not itself idempotent, same hazard as anthropic).
func (s *stream) Close() error {
	var err error
	s.closeOnce.Do(func() { err = s.sdk.Close() })
	return err
}

// mapChunk maps one SDK chunk to (llm.Chunk, emit?). T-4 骨架: Content delta
// → Token; finish_reason via mapFinish. tool_calls accumulation /
// reasoning_content (raw) → T-5.
func mapChunk(c openaisdk.ChatCompletionChunk) (llm.Chunk, bool) {
	if len(c.Choices) == 0 {
		return llm.Chunk{}, false
	}
	choice := c.Choices[0]
	if choice.Delta.Content != "" {
		return llm.Chunk{Token: choice.Delta.Content}, true
	}
	if choice.FinishReason != "" {
		return llm.Chunk{FinishReason: mapFinish(choice.FinishReason)}, true
	}
	return llm.Chunk{}, false
}

// mapFinish maps OpenAI finish_reason → llm.FinishReason. T-4 骨架:
// stop/length. T-5 adds tool_calls→ToolUse / function_call→ToolUse (legacy) /
// content_filter→FinishError (+LLM.CONTENT_FILTERED) / unknown→FinishError
// (原则 3 显式失败, 非 default→Stop).
func mapFinish(fr string) llm.FinishReason {
	switch fr {
	case "stop":
		return llm.FinishStop
	case "length":
		return llm.FinishLength
	default:
		// T-5: tool_calls / function_call / content_filter / unknown 全枚举。
		return llm.FinishStop
	}
}
