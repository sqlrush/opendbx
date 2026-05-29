// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package fake

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sqlrush/opendbx/internal/domain/llm"
)

func validReq() llm.Request {
	return llm.Request{
		Messages:  []llm.Message{{Role: llm.RoleUser, Content: []llm.ContentBlock{{Type: llm.BlockText, Text: "hi"}}}},
		MaxTokens: 100,
	}
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

func TestScripted_TextThenFinish(t *testing.T) {
	t.Parallel()
	p := Scripted("hi", llm.FinishStop)
	s, err := p.Stream(context.Background(), validReq())
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	chunks := drain(t, s)
	if s.Err() != nil {
		t.Errorf("Err = %v; want nil", s.Err())
	}
	// "h", "i", finish
	if len(chunks) != 3 || chunks[0].Token != "h" || chunks[1].Token != "i" || chunks[2].FinishReason != llm.FinishStop {
		t.Errorf("chunks = %+v; want [h i finish:stop]", chunks)
	}
}

func TestProvider_Name(t *testing.T) {
	t.Parallel()
	if New().Name() != "fake" {
		t.Errorf("Name = %q; want fake", New().Name())
	}
}

func TestStartErr_Immediate(t *testing.T) {
	t.Parallel()
	p := New().WithStartErr(llm.ErrAuthFailed)
	_, err := p.Stream(context.Background(), validReq())
	if !errors.Is(err, llm.ErrAuthFailed) {
		t.Errorf("Stream err = %v; want ErrAuthFailed", err)
	}
}

func TestStream_ValidatesRequest(t *testing.T) {
	t.Parallel()
	p := Scripted("x", llm.FinishStop)
	_, err := p.Stream(context.Background(), llm.Request{}) // empty messages
	if !errors.Is(err, llm.ErrRequestInvalid) {
		t.Errorf("Stream err = %v; want ErrRequestInvalid", err)
	}
}

func TestStream_CtxCancelMidStream(t *testing.T) {
	t.Parallel()
	// Long script with delay; cancel after first chunk.
	p := New(
		llm.Chunk{Token: "a"}, llm.Chunk{Token: "b"}, llm.Chunk{Token: "c"},
		llm.Chunk{FinishReason: llm.FinishStop},
	).WithDelay(20 * time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // govet lostcancel: ensure cancel on all paths
	s, err := p.Stream(ctx, validReq())
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer func() { _ = s.Close() }()
	got := 0
	for s.Next() {
		got++
		if got == 1 {
			cancel()
		}
	}
	if !errors.Is(s.Err(), context.Canceled) {
		t.Errorf("Err = %v; want context.Canceled", s.Err())
	}
	if got < 1 {
		t.Errorf("expected ≥1 chunk before cancel; got %d", got)
	}
}

func TestStream_ThinkingAndToolUse(t *testing.T) {
	t.Parallel()
	p := New(
		llm.Chunk{Token: "reasoning", Thinking: true},
		llm.Chunk{FinishReason: llm.FinishToolUse, ToolUses: []llm.ToolUse{{ID: "t1", Name: "topsql", Input: map[string]any{"n": 5}}}},
	)
	s, err := p.Stream(context.Background(), validReq())
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	chunks := drain(t, s)
	if !chunks[0].Thinking {
		t.Errorf("chunk[0].Thinking = false; want true")
	}
	if chunks[1].FinishReason != llm.FinishToolUse || len(chunks[1].ToolUses) != 1 || chunks[1].ToolUses[0].Name != "topsql" {
		t.Errorf("chunk[1] tool-use mismatch: %+v", chunks[1])
	}
}

func TestStream_FinishLength(t *testing.T) {
	t.Parallel()
	p := Scripted("partial", llm.FinishLength)
	s, _ := p.Stream(context.Background(), validReq())
	chunks := drain(t, s)
	if chunks[len(chunks)-1].FinishReason != llm.FinishLength {
		t.Errorf("final finish = %v; want FinishLength", chunks[len(chunks)-1].FinishReason)
	}
}

// TestNewScriptedTurns_AdvancesPerCall guards the spec-1.21 D-7 cursor:
// the Nth Stream call replays turns[N-1], NOT the same turn repeatedly
// (which would silently mask multi-turn loop bugs).
func TestNewScriptedTurns_AdvancesPerCall(t *testing.T) {
	t.Parallel()
	p := NewScriptedTurns(
		Turn{Text: "first", Finish: llm.FinishToolUse, ToolUses: []llm.ToolUse{{ID: "c1", Name: "clock"}}},
		Turn{Text: "second", Finish: llm.FinishStop},
	)

	// Turn 1
	s1, err := p.Stream(context.Background(), validReq())
	if err != nil {
		t.Fatalf("Stream turn 1 err: %v", err)
	}
	c1 := drain(t, s1)
	// One text chunk + one finish chunk.
	if len(c1) != 2 || c1[0].Token != "first" || c1[1].FinishReason != llm.FinishToolUse ||
		len(c1[1].ToolUses) != 1 || c1[1].ToolUses[0].Name != "clock" {
		t.Errorf("turn 1 chunks = %+v; want text=first + tool_use=clock", c1)
	}

	// Turn 2 — cursor advances; we must NOT see "first" again.
	s2, err := p.Stream(context.Background(), validReq())
	if err != nil {
		t.Fatalf("Stream turn 2 err: %v", err)
	}
	c2 := drain(t, s2)
	if len(c2) != 2 || c2[0].Token != "second" || c2[1].FinishReason != llm.FinishStop {
		t.Errorf("turn 2 chunks = %+v; want text=second + FinishStop", c2)
	}

	if got := p.CallCount(); got != 2 {
		t.Errorf("CallCount = %d; want 2", got)
	}
}

// TestNewScriptedTurns_OutOfTurns guards spec-1.21 D-7 explicit failure
// on script exhaustion: silently re-running the last turn would mask
// "test ran longer than scripted" bugs.
func TestNewScriptedTurns_OutOfTurns(t *testing.T) {
	t.Parallel()
	p := NewScriptedTurns(Turn{Text: "only", Finish: llm.FinishStop})

	// First call consumes the single turn.
	if _, err := p.Stream(context.Background(), validReq()); err != nil {
		t.Fatalf("turn 1: %v", err)
	}
	// Second call must surface REQUEST_INVALID.
	_, err := p.Stream(context.Background(), validReq())
	if !errors.Is(err, llm.ErrRequestInvalid) {
		t.Errorf("out-of-turns → %v; want ErrRequestInvalid", err)
	}
}

// TestNewScriptedTurns_EmptyText handles a Turn with no Text — only the
// terminal chunk carrying Finish + ToolUses is emitted (the assistant
// can request a tool without preamble text).
func TestNewScriptedTurns_EmptyText(t *testing.T) {
	t.Parallel()
	p := NewScriptedTurns(Turn{
		Finish:   llm.FinishToolUse,
		ToolUses: []llm.ToolUse{{ID: "x", Name: "echo"}},
	})
	s, _ := p.Stream(context.Background(), validReq())
	chunks := drain(t, s)
	if len(chunks) != 1 || chunks[0].FinishReason != llm.FinishToolUse {
		t.Errorf("empty-text turn = %+v; want 1 chunk with FinishToolUse", chunks)
	}
}
