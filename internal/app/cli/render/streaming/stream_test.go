// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package streaming

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/block"
)

// drainTexts is a test helper extracting Text from each block.Message.
func drainTexts(t *testing.T, s *TokenStream) []string {
	t.Helper()
	blocks := s.Drain()
	out := make([]string, 0, len(blocks))
	for _, b := range blocks {
		if m, ok := b.(block.Message); ok {
			out = append(out, m.Text)
		}
	}
	return out
}

// drainAllMsgs returns block.Message values (typed, ordered).
func drainAllMsgs(t *testing.T, s *TokenStream) []block.Message {
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

// #1 — compile + runtime interface assert.
func TestTokenStream_SatisfiesInterface(t *testing.T) {
	var _ Stream = (*TokenStream)(nil)
	s := NewTokenStream(context.Background())
	_ = s
}

// #2 — single token, no newline → no emit yet.
func TestAppend_SingleToken(t *testing.T) {
	s := NewTokenStream(context.Background())
	if err := s.AppendChunk(Chunk{Token: "hi"}); err != nil {
		t.Fatalf("AppendChunk: %v", err)
	}
	if got := drainTexts(t, s); len(got) != 0 {
		t.Fatalf("Drain empty-line returned %v, want []", got)
	}
}

// #3 — multi-line chunk → multiple block.Message.
func TestAppend_MultilineChunk(t *testing.T) {
	s := NewTokenStream(context.Background())
	if err := s.AppendChunk(Chunk{Token: "a\nb\nc\n"}); err != nil {
		t.Fatalf("AppendChunk: %v", err)
	}
	got := drainTexts(t, s)
	want := []string{"a", "b", "c"}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("Drain got %v, want %v", got, want)
	}
}

// #4 — partial line crossing chunk boundary.
func TestAppend_PartialLineAcrossChunks(t *testing.T) {
	s := NewTokenStream(context.Background())
	_ = s.AppendChunk(Chunk{Token: "foo"})
	_ = s.AppendChunk(Chunk{Token: "bar\n"})
	got := drainTexts(t, s)
	if len(got) != 1 || got[0] != "foobar" {
		t.Fatalf("got %v, want [foobar]", got)
	}
}

// #5 — newline-only chunk → empty-string block.
func TestAppend_NewlineOnlyChunk(t *testing.T) {
	s := NewTokenStream(context.Background())
	_ = s.AppendChunk(Chunk{Token: "\n"})
	got := drainTexts(t, s)
	if len(got) != 1 || got[0] != "" {
		t.Fatalf("got %v, want [\"\"]", got)
	}
}

// #6 — fence open defers emit.
func TestOpenFence_DefersEmit(t *testing.T) {
	s := NewTokenStream(context.Background())
	_ = s.AppendChunk(Chunk{Token: "```code\nlinea"})
	got := drainTexts(t, s)
	if len(got) != 0 {
		t.Fatalf("got %v, want [] (fence still open)", got)
	}
}

// #7 (R2.2 修正 1) — fence close emits whole segment as 1 block.
func TestOpenFence_CloseEmitsWholeSegment(t *testing.T) {
	s := NewTokenStream(context.Background())
	_ = s.AppendChunk(Chunk{Token: "```code\nlinea"})
	_ = s.AppendChunk(Chunk{Token: "\nlineb\n```\n"})
	msgs := drainAllMsgs(t, s)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 whole-fence block, got %d: %+v", len(msgs), msgs)
	}
	if !strings.HasPrefix(msgs[0].Text, "```") {
		t.Fatalf("Text should retain raw opening fence markers, got %q", msgs[0].Text)
	}
	if !strings.Contains(msgs[0].Text, "linea") || !strings.Contains(msgs[0].Text, "lineb") {
		t.Fatalf("Text missing fence body, got %q", msgs[0].Text)
	}
}

// #8 — lineBuf cap forced emit (Continued=true).
func TestLineBufCap_ForcedEmit(t *testing.T) {
	s := NewTokenStream(context.Background(), WithLineBufferCap(16))
	_ = s.AppendChunk(Chunk{Token: strings.Repeat("x", 100)})
	msgs := drainAllMsgs(t, s)
	if len(msgs) == 0 {
		t.Fatal("expected forced-emit block")
	}
	found := false
	for _, m := range msgs {
		if m.Continued {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected at least one Continued=true block; got %+v", msgs)
	}
}

// #10 — empty Drain returns nil.
func TestDrain_EmptyReturnsNil(t *testing.T) {
	s := NewTokenStream(context.Background())
	if got := s.Drain(); got != nil {
		t.Fatalf("empty Drain returned %v, want nil", got)
	}
}

// #11 — multiple chunks → multiple blocks in one Drain.
func TestDrain_MultipleBlocks(t *testing.T) {
	s := NewTokenStream(context.Background())
	for _, tok := range []string{"a\n", "b\n", "c\n", "d\n", "e\n"} {
		_ = s.AppendChunk(Chunk{Token: tok})
	}
	got := drainTexts(t, s)
	if len(got) != 5 {
		t.Fatalf("got %d blocks, want 5: %v", len(got), got)
	}
}

// #12 — FinishStop returns nil err.
func TestFinishStop_NoError(t *testing.T) {
	s := NewTokenStream(context.Background())
	_ = s.AppendChunk(Chunk{Token: "hi\n"})
	_ = s.AppendChunk(Chunk{FinishReason: FinishStop})
	if err := s.Close(); err != nil {
		t.Fatalf("FinishStop Close err=%v, want nil", err)
	}
}

// #13 (痛点 1.1) — FinishLength → ErrStreamTruncated + Truncated marker.
func TestFinishLength_ReturnsErrTruncated(t *testing.T) {
	s := NewTokenStream(context.Background())
	_ = s.AppendChunk(Chunk{Token: "part"})
	_ = s.AppendChunk(Chunk{FinishReason: FinishLength})
	msgs := drainAllMsgs(t, s)
	if len(msgs) != 1 || msgs[0].Text != "part" || !msgs[0].Truncated {
		t.Fatalf("got %+v, want [{Text:part Truncated:true}]", msgs)
	}
	err := s.Close()
	if !errors.Is(err, ErrStreamTruncated) {
		t.Fatalf("Close err=%v, want ErrStreamTruncated", err)
	}
}

// #14 (R2 D5 + R2.1 align) — Err with partial Token processed first.
func TestFinishError_PartialTokenFirst(t *testing.T) {
	s := NewTokenStream(context.Background())
	// errcode-lint:exempt -- spec-1.6 D-7: test mock for Anthropic SSE error event partial-Token path; not production code.
	someErr := errors.New("provider error")
	_ = s.AppendChunk(Chunk{Token: "partial\nfinal", Err: someErr})
	msgs := drainAllMsgs(t, s)
	// Expected: "partial" as a clean line, "final" as truncated partial.
	if len(msgs) != 2 {
		t.Fatalf("got %d blocks, want 2: %+v", len(msgs), msgs)
	}
	if msgs[0].Text != "partial" || msgs[0].Truncated {
		t.Fatalf("first block wrong: %+v", msgs[0])
	}
	if msgs[1].Text != "final" || !msgs[1].Truncated {
		t.Fatalf("second block wrong: %+v", msgs[1])
	}
	err := s.Close()
	if !errors.Is(err, someErr) {
		t.Fatalf("Close err=%v, want wrapped %v", err, someErr)
	}
}

// #15 — FinishContentFilter → ErrStreamFiltered.
func TestFinishContentFilter(t *testing.T) {
	s := NewTokenStream(context.Background())
	_ = s.AppendChunk(Chunk{FinishReason: FinishContentFilter})
	err := s.Close()
	if !errors.Is(err, ErrStreamFiltered) {
		t.Fatalf("Close err=%v, want ErrStreamFiltered", err)
	}
}

// #16 (痛点 1.5) — thinking-only emits Empty placeholder.
func TestThinkingOnly_EmitsEmptyPlaceholder(t *testing.T) {
	s := NewTokenStream(context.Background())
	for i := 0; i < 5; i++ {
		_ = s.AppendChunk(Chunk{Token: ""})
	}
	_ = s.AppendChunk(Chunk{FinishReason: FinishStop})
	_ = s.Close()
	msgs := drainAllMsgs(t, s)
	if len(msgs) != 1 || !msgs[0].Empty {
		t.Fatalf("got %+v, want 1 Empty=true block", msgs)
	}
}

// #17 — ctx cancel mid-stream surfaces ErrStreamCancelled (R2 D1).
func TestCtxCancel_MidStream(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := NewTokenStream(ctx)
	_ = s.AppendChunk(Chunk{Token: "abc\n"})
	cancel()
	// After ctx.Done, next AppendChunk returns ErrStreamCancelled.
	if err := s.AppendChunk(Chunk{Token: "xyz"}); !errors.Is(err, ErrStreamCancelled) {
		t.Fatalf("post-cancel AppendChunk err=%v, want ErrStreamCancelled", err)
	}
	err := s.Close()
	if !errors.Is(err, ErrStreamCancelled) {
		t.Fatalf("Close err=%v, want ErrStreamCancelled", err)
	}
}

// #19 (R2 D3 + R2.1) — Close idempotent + Drain-after-Close stability.
func TestClose_IdempotentDrainAfterClose(t *testing.T) {
	s := NewTokenStream(context.Background())
	_ = s.AppendChunk(Chunk{Token: "hello"}) // partial, no newline
	_ = s.AppendChunk(Chunk{FinishReason: FinishStop})

	// Close × 3: only first cancels; subsequent no-op nil.
	for i := 0; i < 3; i++ {
		err := s.Close()
		if i == 0 && err != nil {
			t.Fatalf("first Close err=%v, want nil for FinishStop", err)
		}
		if i > 0 && err != nil {
			t.Fatalf("Close call %d err=%v, want nil", i+1, err)
		}
	}

	// Drain × 3: first returns final blocks; subsequent return nil.
	first := s.Drain()
	if len(first) == 0 {
		t.Fatal("first Drain after Close returned nil; want final partial block")
	}
	for i := 0; i < 2; i++ {
		if got := s.Drain(); got != nil {
			t.Fatalf("Drain %d after first returned %v, want nil", i+2, got)
		}
	}
}

// #20 — FinishReason getter reflects state.
func TestFinishReason_Getter(t *testing.T) {
	s := NewTokenStream(context.Background())
	if fr := s.FinishReason(); fr != FinishUnset {
		t.Fatalf("initial=%v, want FinishUnset", fr)
	}
	_ = s.AppendChunk(Chunk{FinishReason: FinishStop})
	_ = s.Drain() // triggers processChunkUnsafe
	if fr := s.FinishReason(); fr != FinishStop {
		t.Fatalf("after FinishStop, got %v, want FinishStop", fr)
	}
}

// #21 — Err takes priority over FinishReason in same chunk.
func TestFinishReason_ErrTakesPriority(t *testing.T) {
	s := NewTokenStream(context.Background())
	// errcode-lint:exempt -- spec-1.6 D-7: test mock for Err-priority-over-FinishReason path; not production code.
	someErr := errors.New("oops")
	_ = s.AppendChunk(Chunk{Token: "x\n", Err: someErr, FinishReason: FinishStop})
	_ = s.Drain()
	if fr := s.FinishReason(); fr != FinishError {
		t.Fatalf("got %v, want FinishError (Err priority)", fr)
	}
}

// #22 (R2 D3) — Flush legacy: Append + Flush returns non-nil block.
func TestFlush_LegacyEntry(t *testing.T) {
	s := NewTokenStream(context.Background())
	s.Append("hello") // no newline; partial in lineBuf
	blk, err := s.Flush()
	if err != nil {
		t.Fatalf("Flush err=%v, want nil", err)
	}
	m, ok := blk.(block.Message)
	if !ok || m.Text != "hello" {
		t.Fatalf("Flush block=%+v, want Message{Text:hello}", blk)
	}
}

// #23 (R2 D7) — FinishToolUse propagates signal, no error.
func TestFinishToolUse_NoError(t *testing.T) {
	s := NewTokenStream(context.Background())
	_ = s.AppendChunk(Chunk{Token: "calling tool\n"})
	_ = s.AppendChunk(Chunk{FinishReason: FinishToolUse})
	err := s.Close()
	if err != nil {
		t.Fatalf("FinishToolUse Close err=%v, want nil (caller switches to tool path)", err)
	}
	if fr := s.FinishReason(); fr != FinishToolUse {
		t.Fatalf("got %v, want FinishToolUse", fr)
	}
}

// #24 (R2 D8 + R2.2) — chan full + Close + ctx race.
// Verifies no panic and graceful shutdown under concurrent stress.
func TestRace_ChanFullCloseCtxDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := NewTokenStream(ctx, WithChanCapacity(2))

	const producers = 3
	var wg sync.WaitGroup
	wg.Add(producers)
	for i := 0; i < producers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				_ = s.AppendChunk(Chunk{Token: "x\n"})
			}
		}()
	}

	// Trigger ctx cancel + Close concurrently.
	go func() { cancel() }()
	go func() { _ = s.Close() }()

	wg.Wait()
	// Final state: Close idempotent + Drain works.
	_ = s.Close()
	_ = s.Drain()
}

// #25 (R2 D8 + R2.2) — fence cross-chunk delimiter ("“" + "`").
func TestFence_CrossChunkDelimiter(t *testing.T) {
	s := NewTokenStream(context.Background())
	_ = s.AppendChunk(Chunk{Token: "line1\n``"})
	_ = s.AppendChunk(Chunk{Token: "`code\nbody\n```\n"})
	msgs := drainAllMsgs(t, s)
	// Expected: "line1" (normal line) + 1 fence block whose Text contains body.
	if len(msgs) != 2 {
		t.Fatalf("got %d blocks, want 2: %+v", len(msgs), msgs)
	}
	if msgs[0].Text != "line1" {
		t.Fatalf("first block %q, want line1", msgs[0].Text)
	}
	if !strings.Contains(msgs[1].Text, "body") {
		t.Fatalf("second block missing fence body: %q", msgs[1].Text)
	}
}

// #26 (R2.2 修正 2) — Close pending chunk order: pending chunks emit
// before final partial.
func TestClose_PendingChunkOrder(t *testing.T) {
	s := NewTokenStream(context.Background())
	_ = s.AppendChunk(Chunk{Token: "first\n"})
	_ = s.AppendChunk(Chunk{Token: "second\n"})
	_ = s.AppendChunk(Chunk{FinishReason: FinishStop})
	// No Drain yet; everything is still in s.tokens.
	_ = s.Close()
	got := drainTexts(t, s)
	if len(got) < 2 {
		t.Fatalf("got %v, want at least [first, second, ...] (pending before final)", got)
	}
	if got[0] != "first" || got[1] != "second" {
		t.Fatalf("order wrong: got %v, want [first, second, ...]", got)
	}
}
