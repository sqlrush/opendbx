// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package anthropic

import (
	"bytes"
	"encoding/json"
	"sort"
	"sync"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"

	"github.com/sqlrush/opendbx/internal/domain/llm"
)

// JSON guard bounds for tool_use input decode (D-4 / R2 MED-1).
const (
	maxToolInputBytes = 256 * 1024 // 256 KB per tool input
	maxToolInputDepth = 32
	// maxToolBlocks caps concurrent tool_use blocks per turn (T-10a security
	// HIGH-2): an unbounded toolAcc map lets a hostile/confused model
	// allocate N × maxToolInputBytes of accumulation buffers. Anthropic's
	// practical per-turn tool count is well under this.
	maxToolBlocks = 64
)

// stream wraps the SDK SSE stream as an llm.Stream iterator. Next loops
// SDK events until it produces one llm.Chunk (some events — message_start,
// content_block_start, citations/signature_delta — produce none).
type stream struct {
	sdk       *ssestream.Stream[anthropicsdk.MessageStreamEventUnion]
	cur       llm.Chunk
	err       error
	done      bool
	toolAcc   map[int64]*toolBlock // content-block index → accumulating tool_use
	closeOnce sync.Once            // T-10a MED-4: SDK Close is not idempotent
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
		// mapEvent may set s.err on a hard guard violation (oversize tool
		// input / too many tool blocks). Stop the SDK loop so s.Err()
		// surfaces it via the consumeStream terminal path (T-10a HIGH-2).
		if s.err != nil {
			s.done = true
			return false
		}
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
			if len(s.toolAcc) >= maxToolBlocks {
				s.err = llm.ErrDecodeFailed // T-10a HIGH-2: cap concurrent tool blocks
				return llm.Chunk{}, false
			}
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
				// T-10a HIGH-2: bound accumulation BEFORE the write so a
				// single huge input_json_delta cannot allocate unboundedly
				// (the 256 KB guard in decodeToolInput fires too late — only
				// at message_delta, after the buffer already grew).
				if tb.buf.Len()+len(ev.Delta.PartialJSON) > maxToolInputBytes {
					s.err = llm.ErrDecodeFailed
					return llm.Chunk{}, false
				}
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
	// T-10a HIGH-2/MED-3: iterate by ascending content-block index so the
	// ToolUse slice order is deterministic (map iteration is random). The
	// index carries protocol meaning that spec-1.21's executor relies on.
	keys := make([]int64, 0, len(s.toolAcc))
	for k := range s.toolAcc {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	out := make([]llm.ToolUse, 0, len(keys))
	for _, k := range keys {
		tb := s.toolAcc[k]
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

// Close releases the SDK stream. Idempotent via sync.Once — the SDK's
// ssestream.Stream.Close (v1.45.0) is NOT itself idempotent (a second call
// reaches http.Response.Body.Close twice), and this stream is closed by
// both consumeStream's defer and any drain helper's defer (T-10a MED-4).
func (s *stream) Close() error {
	var err error
	s.closeOnce.Do(func() { err = s.sdk.Close() })
	return err
}

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
	// T-10a MED: an unrecognized stop_reason (a future Anthropic addition)
	// maps to FinishStop deliberately — degrade gracefully to a clean end
	// rather than break the stream. A new value worth distinguishing is
	// caught by the TestMapStopReason table + an SDK version bump, not
	// silently in production (spec-1.20 R2 H-6 enum is the source of truth).
	return llm.FinishStop
}
