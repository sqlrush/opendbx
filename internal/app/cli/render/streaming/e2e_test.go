// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package streaming

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/sqlrush/opendbx/internal/app/cli/render/block"
)

// fakeSSESource simulates an Anthropic-style SSE event stream per spec
// § 4.2 R2 D9. Tests inject ordered events (content / finish / error /
// cancel) and the source feeds them into a TokenStream.
type sseEventType int

const (
	sseContent sseEventType = iota // payload = token string
	sseFinish                      // payload = FinishReason
	sseError                       // payload = (token string, err error)
	sseCancel                      // sentinel; test triggers ctx.cancel here
)

type sseEvent struct {
	typ    sseEventType
	token  string
	reason FinishReason
	err    error
	delay  time.Duration
}

type fakeSSESource struct {
	events []sseEvent
}

// Run drives events into the stream. cancelFn is invoked when an
// sseCancel event is reached.
func (f *fakeSSESource) Run(ctx context.Context, s *TokenStream, cancelFn context.CancelFunc) {
	for _, e := range f.events {
		if e.delay > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(e.delay):
			}
		}
		switch e.typ {
		case sseContent:
			if err := s.AppendChunk(Chunk{Token: e.token}); err != nil {
				return
			}
		case sseFinish:
			_ = s.AppendChunk(Chunk{FinishReason: e.reason})
		case sseError:
			_ = s.AppendChunk(Chunk{Token: e.token, Err: e.err})
		case sseCancel:
			cancelFn()
			return
		}
	}
}

// E2E-1a (痛点 1.1 第 1 入口 FinishLength).
func TestE2E_FinishLength(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := NewTokenStream(ctx)

	src := &fakeSSESource{events: []sseEvent{
		{typ: sseContent, token: "Hello, "},
		{typ: sseContent, token: "this is a "},
		{typ: sseContent, token: "long output that "},
		{typ: sseContent, token: "got truncated"},
		{typ: sseFinish, reason: FinishLength},
	}}
	src.Run(ctx, s, cancel)

	err := s.Close()
	if !errors.Is(err, ErrStreamTruncated) {
		t.Fatalf("Close err=%v, want ErrStreamTruncated", err)
	}
	msgs := drainE2EMsgs(t, s)
	if len(msgs) == 0 {
		t.Fatal("expected at least 1 block")
	}
	last := msgs[len(msgs)-1]
	if !last.Truncated {
		t.Fatalf("last block Truncated=false; want true. last=%+v", last)
	}
	// Hint should be actionable.
	if !strings.Contains(ErrStreamTruncated.Hint(), "拆分") &&
		!strings.Contains(ErrStreamTruncated.Hint(), "max_tokens") {
		t.Errorf("hint not actionable: %q", ErrStreamTruncated.Hint())
	}
}

// E2E-1b (痛点 1.1 第 2 入口 Chunk.Err with partial Token).
func TestE2E_ChunkErrWithPartial(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := NewTokenStream(ctx)

	// errcode-lint:exempt -- spec-1.6 D-7: E2E test mock for upstream LLM provider error path; not production code.
	provErr := errors.New("provider connection closed")
	src := &fakeSSESource{events: []sseEvent{
		{typ: sseContent, token: "Begin "},
		{typ: sseContent, token: "response\n"},
		{typ: sseError, token: "partial\nfinal", err: provErr},
	}}
	src.Run(ctx, s, cancel)

	err := s.Close()
	if !errors.Is(err, provErr) {
		t.Fatalf("Close err=%v, want wrapped %v", err, provErr)
	}
	msgs := drainE2EMsgs(t, s)
	// Expect: "Begin response", "partial" (clean line from error chunk),
	// "final" (truncated partial).
	if len(msgs) < 3 {
		t.Fatalf("got %d msgs, want >=3 (response/partial/final): %+v", len(msgs), msgs)
	}
	// Verify "partial" was processed BEFORE error caused truncation.
	foundPartial := false
	for _, m := range msgs {
		if m.Text == "partial" && !m.Truncated {
			foundPartial = true
		}
	}
	if !foundPartial {
		t.Errorf("Err path did not process partial Token before error; got %+v", msgs)
	}
}

// E2E-1c (痛点 1.1 第 3 入口 ctx.Done mid-stream).
func TestE2E_CtxCancelMidStream(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := NewTokenStream(ctx)

	src := &fakeSSESource{events: []sseEvent{
		{typ: sseContent, token: "partial "},
		{typ: sseContent, token: "before "},
		{typ: sseContent, token: "cancel"},
		{typ: sseCancel}, // triggers cancel()
	}}
	src.Run(ctx, s, cancel)

	// Next AppendChunk should fail.
	if err := s.AppendChunk(Chunk{Token: "ignored"}); !errors.Is(err, ErrStreamCancelled) {
		t.Errorf("post-cancel AppendChunk err=%v, want ErrStreamCancelled", err)
	}

	err := s.Close()
	if !errors.Is(err, ErrStreamCancelled) {
		t.Fatalf("Close err=%v, want ErrStreamCancelled", err)
	}
	msgs := drainE2EMsgs(t, s)
	if len(msgs) == 0 {
		t.Fatal("expected at least 1 partial block after cancel")
	}
	last := msgs[len(msgs)-1]
	if !last.Truncated {
		t.Errorf("ctx-cancel final partial Truncated=false; want true. last=%+v", last)
	}
}

// E2E-2 (痛点 1.5 thinking-only).
func TestE2E_ThinkingOnly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := NewTokenStream(ctx)

	src := &fakeSSESource{events: []sseEvent{
		{typ: sseContent, token: ""},
		{typ: sseContent, token: ""},
		{typ: sseContent, token: ""},
		{typ: sseFinish, reason: FinishStop},
	}}
	src.Run(ctx, s, cancel)
	_ = s.Close()

	msgs := drainE2EMsgs(t, s)
	if len(msgs) != 1 || !msgs[0].Empty {
		t.Fatalf("got %+v, want 1 Empty=true placeholder", msgs)
	}
}

// E2E-3 — full chain: Stream → block.Message → caller.Push (mocked).
// Mimics spec § 3.3 walkthrough wiring without depending on scrollback
// (DAG: streaming index 9 does NOT import scrollback).
func TestE2E_FullChainToCaller(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := NewTokenStream(ctx)

	src := &fakeSSESource{events: []sseEvent{
		{typ: sseContent, token: "line A\n"},
		{typ: sseContent, token: "line B\n"},
		{typ: sseContent, token: "line C\n"},
		{typ: sseFinish, reason: FinishStop},
	}}
	src.Run(ctx, s, cancel)

	// Simulate caller's render loop: collect Drain output across calls.
	var collected []block.RenderNode
	collected = append(collected, s.Drain()...)
	_ = s.Close()
	collected = append(collected, s.Drain()...)

	if len(collected) < 3 {
		t.Fatalf("got %d blocks, want >=3 (line A/B/C): %+v", len(collected), collected)
	}
	// First 3 blocks should be the clean lines.
	want := []string{"line A", "line B", "line C"}
	for i := 0; i < 3; i++ {
		m, ok := collected[i].(block.Message)
		if !ok || m.Text != want[i] {
			t.Errorf("collected[%d]=%+v, want Message{Text:%q}", i, collected[i], want[i])
		}
	}
}

// drainE2EMsgs is a local helper (different from stream_test.go's
// drainAllMsgs to avoid cross-file dependency).
func drainE2EMsgs(t *testing.T, s *TokenStream) []block.Message {
	t.Helper()
	blocks := s.Drain()
	out := make([]block.Message, 0, len(blocks))
	for _, b := range blocks {
		if m, ok := b.(block.Message); ok {
			out = append(out, m)
		}
	}
	return out
}
