// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package anthropic

import (
	"bytes"
	"errors"
	"slices"
	"sync"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"

	"github.com/sqlrush/opendbx/internal/domain/llm"
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
			if len(s.toolAcc) >= llm.MaxToolBlocks {
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
				// (the 256 KB guard in llm.DecodeToolInput fires too late — only
				// at message_delta, after the buffer already grew).
				if tb.buf.Len()+len(ev.Delta.PartialJSON) > llm.MaxToolInputBytes {
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
	slices.Sort(keys)
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

// Chunk returns the current chunk.
func (s *stream) Chunk() llm.Chunk { return s.cur }

// Err returns the terminal error, mapped to a registered LLM.* errcode
// (T-10a HIGH-3). A raw SDK transport/API error (e.g. 401 invalid key in
// the stream phase) must not escape the adapter un-classified (原则 3 +
// 规则 7 + 规则 16).
func (s *stream) Err() error { return classifyStreamErr(s.err) }

// classifyStreamErr maps a raw SDK / transport error to a registered LLM.*
// errcode by HTTP status. Non-SDK errors (context cancel/deadline, our own
// ErrDecodeFailed from the tool-input guard) pass through unchanged — ctx
// errors are classified by the app-layer finishFromErr, and ErrDecodeFailed
// is already a registered code.
func classifyStreamErr(err error) error {
	if err == nil {
		return nil
	}
	var apiErr *anthropicsdk.Error
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case 400, 422:
			return llm.ErrRequestInvalid
		case 401, 403:
			return llm.ErrAuthFailed
		case 408:
			return llm.ErrTimeout
		default:
			// 429 rate-limit / 5xx / 529 overloaded / unknown → unavailable.
			return llm.ErrUnavailable
		}
	}
	return err
}

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
