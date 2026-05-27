// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package anthropic

import (
	"bytes"
	"encoding/json"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"

	"github.com/sqlrush/opendbx/internal/domain/llm"
)

// JSON guard bounds for tool_use input decode (D-4 / R2 MED-1).
const (
	maxToolInputBytes = 256 * 1024 // 256 KB
	maxToolInputDepth = 32
)

// stream wraps the SDK SSE stream as an llm.Stream iterator. Next loops
// SDK events until it produces one llm.Chunk (some events — message_start,
// content_block_start, citations/signature_delta — produce none).
type stream struct {
	sdk     *ssestream.Stream[anthropicsdk.MessageStreamEventUnion]
	cur     llm.Chunk
	err     error
	done    bool
	toolAcc map[int64]*toolBlock // content-block index → accumulating tool_use
}

type toolBlock struct {
	id   string
	name string
	buf  bytes.Buffer // input_json_delta partial JSON accumulation
}

func newStream(sdk *ssestream.Stream[anthropicsdk.MessageStreamEventUnion]) *stream {
	return &stream{sdk: sdk, toolAcc: map[int64]*toolBlock{}}
}

// Next advances to the next emittable llm.Chunk. Returns false at end /
// error. Honors the SDK stream's own ctx handling (Err surfaces ctx err).
func (s *stream) Next() bool {
	if s.done || s.err != nil {
		return false
	}
	for s.sdk.Next() {
		ev := s.sdk.Current()
		chunk, emit := s.mapEvent(ev)
		if emit {
			s.cur = chunk
			return true
		}
		// event produced no chunk (start/stop/citations/signature) — keep looping.
	}
	// SDK stream ended.
	if err := s.sdk.Err(); err != nil {
		s.err = err
	}
	s.done = true
	return false
}

// mapEvent maps one SDK event to (chunk, emit?). emit=false means skip.
func (s *stream) mapEvent(ev anthropicsdk.MessageStreamEventUnion) (llm.Chunk, bool) {
	switch ev.Type {
	case "content_block_start":
		// Record tool_use blocks so input_json_delta can accumulate.
		cb := ev.ContentBlock
		if cb.Type == "tool_use" {
			s.toolAcc[ev.Index] = &toolBlock{id: cb.ID, name: cb.Name}
		}
		return llm.Chunk{}, false

	case "content_block_delta":
		switch ev.Delta.Type {
		case "text_delta":
			return llm.Chunk{Token: ev.Delta.Text}, true
		case "thinking_delta":
			return llm.Chunk{Token: ev.Delta.Thinking, Thinking: true}, true
		case "input_json_delta":
			if tb := s.toolAcc[ev.Index]; tb != nil {
				tb.buf.WriteString(ev.Delta.PartialJSON)
			}
			return llm.Chunk{}, false
		default:
			// citations_delta / signature_delta — ignored (❌-14).
			return llm.Chunk{}, false
		}

	case "message_delta":
		fr := mapStopReason(anthropicsdk.StopReason(ev.Delta.StopReason))
		chunk := llm.Chunk{FinishReason: fr}
		if fr == llm.FinishToolUse {
			tools, err := s.decodeTools()
			if err != nil {
				return llm.Chunk{FinishReason: llm.FinishError, Err: err}, true
			}
			chunk.ToolUses = tools
		}
		return chunk, true

	default:
		// message_start / content_block_stop / message_stop — no chunk.
		return llm.Chunk{}, false
	}
}

// decodeTools decodes each accumulated tool_use input with JSON guard
// (object / depth / size; UseNumber). R2 MED-1 / D-4.
func (s *stream) decodeTools() ([]llm.ToolUse, error) {
	if len(s.toolAcc) == 0 {
		return nil, nil
	}
	out := make([]llm.ToolUse, 0, len(s.toolAcc))
	for _, tb := range s.toolAcc {
		input, err := decodeToolInput(tb.buf.Bytes())
		if err != nil {
			return nil, err
		}
		out = append(out, llm.ToolUse{ID: tb.id, Name: tb.name, Input: input})
	}
	return out, nil
}

// decodeToolInput decodes tool input JSON with object/size/depth guard.
func decodeToolInput(raw []byte) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil // empty input → empty object
	}
	if len(raw) > maxToolInputBytes {
		return nil, llm.ErrDecodeFailed
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, llm.ErrDecodeFailed
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return nil, llm.ErrDecodeFailed // tool input must be a JSON object
	}
	if jsonDepth(v, 1) > maxToolInputDepth {
		return nil, llm.ErrDecodeFailed
	}
	return obj, nil
}

// jsonDepth returns the maximum nesting depth of a decoded JSON value.
func jsonDepth(v any, cur int) int {
	max := cur
	switch t := v.(type) {
	case map[string]any:
		for _, child := range t {
			if d := jsonDepth(child, cur+1); d > max {
				max = d
			}
		}
	case []any:
		for _, child := range t {
			if d := jsonDepth(child, cur+1); d > max {
				max = d
			}
		}
	}
	return max
}

// Chunk returns the current chunk.
func (s *stream) Chunk() llm.Chunk { return s.cur }

// Err returns the terminal error (SDK stream error, incl. ctx err).
func (s *stream) Err() error { return s.err }

// Close releases the SDK stream (idempotent).
func (s *stream) Close() error { return s.sdk.Close() }

// mapStopReason maps Anthropic stop_reason to the full llm.FinishReason
// enum (R2 H-6). Empty stop_reason (mid-stream message_delta) → FinishStop
// is wrong; an empty value maps to FinishUnset so the caller does not
// prematurely terminate. Anthropic sends stop_reason only on the final
// message_delta.
func mapStopReason(sr anthropicsdk.StopReason) llm.FinishReason {
	switch sr {
	case anthropicsdk.StopReasonEndTurn:
		return llm.FinishStop
	case anthropicsdk.StopReasonMaxTokens:
		return llm.FinishLength
	case anthropicsdk.StopReasonToolUse:
		return llm.FinishToolUse
	case anthropicsdk.StopReasonStopSequence:
		return llm.FinishStopSequence
	case anthropicsdk.StopReasonPauseTurn:
		return llm.FinishPause
	case anthropicsdk.StopReasonRefusal:
		return llm.FinishRefusal
	case "":
		return llm.FinishUnset
	}
	return llm.FinishStop
}
