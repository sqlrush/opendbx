// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package openai

import (
	"errors"
	"testing"

	openaisdk "github.com/openai/openai-go"
	"github.com/openai/openai-go/packages/ssestream"

	"github.com/sqlrush/opendbx/internal/domain/llm"
)

// fakeDecoder implements ssestream.Decoder, yielding canned chunk JSON so the
// real *ssestream.Stream iterator (and our stream wrapper) can be driven
// without a network round-trip (mirrors the anthropic adapter test).
type fakeDecoder struct {
	events []string
	idx    int
	cur    ssestream.Event
	err    error
}

func (d *fakeDecoder) Next() bool {
	if d.idx >= len(d.events) {
		return false
	}
	d.cur = ssestream.Event{Data: []byte(d.events[d.idx])} // openai SSE is data-only
	d.idx++
	return true
}
func (d *fakeDecoder) Event() ssestream.Event { return d.cur }
func (d *fakeDecoder) Close() error           { return nil }
func (d *fakeDecoder) Err() error             { return d.err }

func newTestStream(events []string, decErr error) *stream {
	dec := &fakeDecoder{events: events, err: decErr}
	sdk := ssestream.NewStream[openaisdk.ChatCompletionChunk](dec, nil)
	return newStream(sdk)
}

func drain(t *testing.T, s llm.Stream) []llm.Chunk {
	t.Helper()
	defer func() { _ = s.Close() }()
	var out []llm.Chunk
	for s.Next() {
		out = append(out, s.Chunk())
	}
	return out
}

func TestStreamIterator_TextThenStop(t *testing.T) {
	t.Parallel()
	s := newTestStream([]string{
		`{"choices":[{"delta":{"content":"Hello"}}]}`,
		`{"choices":[{"delta":{"content":" world"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	}, nil)
	chunks := drain(t, s)
	if s.Err() != nil {
		t.Fatalf("Err = %v", s.Err())
	}
	if len(chunks) != 3 {
		t.Fatalf("chunks = %d (%+v); want 3", len(chunks), chunks)
	}
	if chunks[0].Token != "Hello" || chunks[1].Token != " world" {
		t.Errorf("text chunks = %q,%q", chunks[0].Token, chunks[1].Token)
	}
	if chunks[2].FinishReason != llm.FinishStop {
		t.Errorf("final = %v; want FinishStop", chunks[2].FinishReason)
	}
}

func TestStreamIterator_ToolCalls(t *testing.T) {
	t.Parallel()
	s := newTestStream([]string{
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"topsql","arguments":""}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"n\": 5}"}}]}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
	}, nil)
	chunks := drain(t, s)
	last := chunks[len(chunks)-1]
	if last.FinishReason != llm.FinishToolUse || len(last.ToolUses) != 1 || last.ToolUses[0].Name != "topsql" {
		t.Errorf("tool-call stream final = %+v; want FinishToolUse topsql", last)
	}
	if last.ToolUses[0].ID != "call_1" {
		t.Errorf("tool ID = %q; want call_1", last.ToolUses[0].ID)
	}
}

func TestStreamIterator_DecoderError(t *testing.T) {
	t.Parallel()
	s := newTestStream([]string{
		`{"choices":[{"delta":{"content":"partial"}}]}`,
	}, errors.New("network boom"))
	_ = drain(t, s)
	if s.Err() == nil {
		t.Errorf("expected terminal Err from decoder; got nil")
	}
}
