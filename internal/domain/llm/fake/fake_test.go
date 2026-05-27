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
