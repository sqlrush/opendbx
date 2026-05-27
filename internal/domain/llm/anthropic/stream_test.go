// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package anthropic

import (
	"errors"
	"testing"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"

	"github.com/sqlrush/opendbx/internal/domain/llm"
)

// fakeDecoder implements ssestream.Decoder, yielding canned SSE events so
// the real *ssestream.Stream iterator (and our stream wrapper) can be
// driven without a network round-trip (mirrors the SDK's own test style).
type fakeDecoder struct {
	events []string
	idx    int
	cur    ssestream.Event
	err    error
	closed bool
}

func (d *fakeDecoder) Next() bool {
	if d.idx >= len(d.events) {
		return false
	}
	// Event.Type drives ssestream.Stream's switch; Data is the full event JSON.
	d.cur = ssestream.Event{Type: eventType(d.events[d.idx]), Data: []byte(d.events[d.idx])}
	d.idx++
	return true
}
func (d *fakeDecoder) Event() ssestream.Event { return d.cur }
func (d *fakeDecoder) Close() error           { d.closed = true; return nil }
func (d *fakeDecoder) Err() error             { return d.err }

// eventType extracts the "type" field cheaply (events are small canned JSON).
func eventType(s string) string {
	// crude but sufficient for test fixtures of the form {"type":"X",...}
	const key = `"type":"`
	i := indexOf(s, key)
	if i < 0 {
		return ""
	}
	rest := s[i+len(key):]
	j := indexOf(rest, `"`)
	if j < 0 {
		return ""
	}
	return rest[:j]
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func newTestStream(events []string, decErr error) *stream {
	dec := &fakeDecoder{events: events, err: decErr}
	sdk := ssestream.NewStream[anthropicsdk.MessageStreamEventUnion](dec, nil)
	return newStream(sdk)
}

func drainStream(t *testing.T, s llm.Stream) []llm.Chunk {
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
		`{"type":"message_start","message":{}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{}}`,
		`{"type":"message_stop"}`,
	}, nil)
	chunks := drainStream(t, s)
	if s.Err() != nil {
		t.Fatalf("Err = %v", s.Err())
	}
	// "Hello", " world", FinishStop (message_start/block_start/stop/message_stop emit nothing)
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

func TestStreamIterator_ToolUse(t *testing.T) {
	t.Parallel()
	s := newTestStream([]string{
		`{"type":"message_start","message":{}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"topsql","input":{}}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"n\": 5}"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{}}`,
		`{"type":"message_stop"}`,
	}, nil)
	chunks := drainStream(t, s)
	last := chunks[len(chunks)-1]
	if last.FinishReason != llm.FinishToolUse || len(last.ToolUses) != 1 || last.ToolUses[0].Name != "topsql" {
		t.Errorf("tool-use stream final = %+v; want FinishToolUse topsql", last)
	}
}

func TestStreamIterator_DecoderError(t *testing.T) {
	t.Parallel()
	s := newTestStream([]string{
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"partial"}}`,
	}, errors.New("network boom"))
	_ = drainStream(t, s)
	// decErr is set on the decoder; ssestream surfaces it after events drain.
	if s.Err() == nil {
		t.Errorf("expected terminal Err from decoder; got nil")
	}
}
