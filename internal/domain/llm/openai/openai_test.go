// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package openai

import (
	"errors"
	"testing"

	openaisdk "github.com/openai/openai-go"

	"github.com/sqlrush/opendbx/internal/domain/llm"
)

func TestNew_MissingKey(t *testing.T) {
	t.Parallel()
	_, err := New(Config{Model: "m", BaseURL: "http://x/v1"})
	if !errors.Is(err, llm.ErrAuthFailed) {
		t.Errorf("missing key → %v; want ErrAuthFailed", err)
	}
}

func TestNew_KeylessOllama(t *testing.T) {
	t.Parallel()
	p, err := New(Config{Model: "qwen", BaseURL: "http://localhost:11434/v1", Name: "ollama", AllowKeyless: true})
	if err != nil || p == nil {
		t.Fatalf("keyless ollama New: %v", err)
	}
	if p.Name() != "ollama" {
		t.Errorf("Name = %q; want ollama", p.Name())
	}
}

func TestNew_DefaultName(t *testing.T) {
	t.Parallel()
	p, _ := New(Config{APIKey: "sk", Model: "m", BaseURL: "http://x/v1"})
	if p.Name() != "openai-compat" {
		t.Errorf("Name = %q; want openai-compat", p.Name())
	}
}

func TestStream_ValidatesRequest(t *testing.T) {
	t.Parallel()
	p, _ := New(Config{APIKey: "sk", Model: "m", BaseURL: "http://x/v1"})
	_, err := p.Stream(t.Context(), llm.Request{}) // empty messages → fails before SDK call
	if !errors.Is(err, llm.ErrRequestInvalid) {
		t.Errorf("invalid req → %v; want ErrRequestInvalid", err)
	}
}

func TestJoinSystem(t *testing.T) {
	t.Parallel()
	got := joinSystem([]llm.SystemBlock{{Text: "a"}, {Text: ""}, {Text: "b"}})
	if got != "a\n\nb" { // \n\n separator, skip empty (spec-1.20.1 Q2)
		t.Errorf("joinSystem = %q; want %q", got, "a\n\nb")
	}
	if joinSystem(nil) != "" {
		t.Errorf("joinSystem(nil) should be empty")
	}
}

func TestFirstText(t *testing.T) {
	t.Parallel()
	if got := firstText([]llm.ContentBlock{{Type: llm.BlockText, Text: "hi"}}); got != "hi" {
		t.Errorf("firstText = %q; want hi", got)
	}
}

func TestMapFinish(t *testing.T) {
	t.Parallel()
	// T-4 骨架: stop/length only (tool_calls/function_call/content_filter/unknown → T-5).
	cases := map[string]llm.FinishReason{
		"stop":   llm.FinishStop,
		"length": llm.FinishLength,
		"":       llm.FinishStop,
	}
	for in, want := range cases {
		if got := mapFinish(in); got != want {
			t.Errorf("mapFinish(%q) = %v; want %v", in, got, want)
		}
	}
}

func TestMapChunk(t *testing.T) {
	t.Parallel()
	// content delta → Token
	c := openaisdk.ChatCompletionChunk{Choices: []openaisdk.ChatCompletionChunkChoice{
		{Delta: openaisdk.ChatCompletionChunkChoiceDelta{Content: "hello"}},
	}}
	if chunk, emit := mapChunk(c); !emit || chunk.Token != "hello" {
		t.Errorf("content chunk → %+v emit=%v; want Token=hello", chunk, emit)
	}
	// finish_reason stop → FinishStop
	f := openaisdk.ChatCompletionChunk{Choices: []openaisdk.ChatCompletionChunkChoice{
		{FinishReason: "stop"},
	}}
	if chunk, emit := mapChunk(f); !emit || chunk.FinishReason != llm.FinishStop {
		t.Errorf("finish chunk → %+v emit=%v; want FinishStop", chunk, emit)
	}
	// empty choices → no emit
	if _, emit := mapChunk(openaisdk.ChatCompletionChunk{}); emit {
		t.Errorf("empty choices should not emit")
	}
}
