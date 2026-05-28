// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package anthropic

import (
	"errors"
	"strconv"
	"strings"
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
	i := strings.Index(s, key)
	if i < 0 {
		return ""
	}
	rest := s[i+len(key):]
	j := strings.Index(rest, `"`)
	if j < 0 {
		return ""
	}
	return rest[:j]
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

// TestClassifyStreamErr is the T-10a HIGH-3 regression: a raw SDK API error
// must be mapped to a registered LLM.* errcode by HTTP status (原则 3 — no
// raw SDK error escapes the adapter), while non-SDK errors pass through.
func TestClassifyStreamErr(t *testing.T) {
	t.Parallel()
	cases := []struct {
		status int
		want   error
	}{
		{400, llm.ErrRequestInvalid},
		{422, llm.ErrRequestInvalid},
		{401, llm.ErrAuthFailed},
		{403, llm.ErrAuthFailed},
		{408, llm.ErrTimeout},
		{429, llm.ErrUnavailable},
		{500, llm.ErrUnavailable},
		{529, llm.ErrUnavailable},
	}
	for _, tc := range cases {
		got := classifyStreamErr(&anthropicsdk.Error{StatusCode: tc.status})
		if !errors.Is(got, tc.want) {
			t.Errorf("status %d → %v; want %v", tc.status, got, tc.want)
		}
	}
	if classifyStreamErr(nil) != nil {
		t.Errorf("nil should map to nil")
	}
	// Non-SDK error passes through unchanged (ctx errors / ErrDecodeFailed).
	passthrough := errors.New("ctx boom")
	if got := classifyStreamErr(passthrough); !errors.Is(got, passthrough) {
		t.Errorf("non-SDK error should pass through unchanged; got %v", got)
	}
	if !errors.Is(classifyStreamErr(llm.ErrDecodeFailed), llm.ErrDecodeFailed) {
		t.Errorf("ErrDecodeFailed should pass through")
	}
}

// TestStreamIterator_OversizeToolInputBuffer is the T-10a HIGH-2 regression:
// a single huge input_json_delta must be rejected DURING accumulation
// (before the buffer grows unbounded), surfacing ErrDecodeFailed and
// stopping the stream — not only at message_delta decode time.
func TestStreamIterator_OversizeToolInputBuffer(t *testing.T) {
	t.Parallel()
	huge := strings.Repeat("a", llm.MaxToolInputBytes+10)
	s := newTestStream([]string{
		`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"topsql","input":{}}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"` + huge + `"}}`,
		`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{}}`,
	}, nil)
	_ = drainStream(t, s)
	if !errors.Is(s.Err(), llm.ErrDecodeFailed) {
		t.Errorf("oversize accumulation should set ErrDecodeFailed; got %v", s.Err())
	}
}

// TestStreamIterator_TooManyToolBlocks is the T-10a HIGH-2 cap on concurrent
// tool blocks.
func TestStreamIterator_TooManyToolBlocks(t *testing.T) {
	t.Parallel()
	events := make([]string, 0, llm.MaxToolBlocks+2)
	for i := 0; i <= llm.MaxToolBlocks; i++ { // llm.MaxToolBlocks+1 starts → over the cap
		events = append(events,
			`{"type":"content_block_start","index":`+strconv.Itoa(i)+`,"content_block":{"type":"tool_use","id":"t","name":"n","input":{}}}`)
	}
	s := newTestStream(events, nil)
	_ = drainStream(t, s)
	if !errors.Is(s.Err(), llm.ErrDecodeFailed) {
		t.Errorf("exceeding llm.MaxToolBlocks should set ErrDecodeFailed; got %v", s.Err())
	}
}

// TestStreamIterator_ToolOrderDeterministic is the T-10a MED-3 regression:
// tool blocks started out of index order must decode in ascending index
// order (map iteration is random).
func TestStreamIterator_ToolOrderDeterministic(t *testing.T) {
	t.Parallel()
	// Start blocks in reverse index order 2,1,0 with names t2,t1,t0.
	s := newTestStream([]string{
		`{"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"i2","name":"t2","input":{}}}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"i1","name":"t1","input":{}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"i0","name":"t0","input":{}}}`,
		`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{}}`,
	}, nil)
	chunks := drainStream(t, s)
	last := chunks[len(chunks)-1]
	if len(last.ToolUses) != 3 {
		t.Fatalf("want 3 tools; got %+v", last.ToolUses)
	}
	for i, want := range []string{"t0", "t1", "t2"} {
		if last.ToolUses[i].Name != want {
			t.Errorf("ToolUses[%d].Name = %q; want %q (ascending index order)", i, last.ToolUses[i].Name, want)
		}
	}
}
