// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package openai

import (
	"encoding/json"
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
	if got := joinSystem([]llm.SystemBlock{{Text: "a"}, {Text: ""}, {Text: "b"}}); got != "a\n\nb" {
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

// TestMapFinish covers the full OpenAI finish_reason enum (spec-1.20.1 Q10).
func TestMapFinish(t *testing.T) {
	t.Parallel()
	cases := []struct {
		fr      string
		want    llm.FinishReason
		wantErr error
	}{
		{"stop", llm.FinishStop, nil},
		{"length", llm.FinishLength, nil},
		{"tool_calls", llm.FinishToolUse, nil},
		{"function_call", llm.FinishToolUse, nil}, // legacy Functions API
		{"content_filter", llm.FinishError, llm.ErrContentFiltered},
		{"weird_future", llm.FinishError, llm.ErrDecodeFailed}, // unknown → Error (原则 3)
	}
	for _, tc := range cases {
		chunk, emit := newStream(nil).mapFinish(tc.fr)
		if !emit || chunk.FinishReason != tc.want {
			t.Errorf("mapFinish(%q) = %v emit=%v; want %v", tc.fr, chunk.FinishReason, emit, tc.want)
		}
		if tc.wantErr != nil && !errors.Is(chunk.Err, tc.wantErr) {
			t.Errorf("mapFinish(%q) Err = %v; want %v", tc.fr, chunk.Err, tc.wantErr)
		}
	}
}

func TestMapChunk_Content(t *testing.T) {
	t.Parallel()
	c := openaisdk.ChatCompletionChunk{Choices: []openaisdk.ChatCompletionChunkChoice{
		{Delta: openaisdk.ChatCompletionChunkChoiceDelta{Content: "hello"}},
	}}
	chunk, emit := newStream(nil).mapChunk(c)
	if !emit || chunk.Token != "hello" || chunk.Thinking {
		t.Errorf("content → %+v emit=%v; want Token=hello", chunk, emit)
	}
	// empty choices → no emit
	if _, emit := newStream(nil).mapChunk(openaisdk.ChatCompletionChunk{}); emit {
		t.Errorf("empty choices should not emit")
	}
}

// TestMapChunk_Reasoning verifies reasoning_content (vendor raw extra field,
// B-64 无 typed) → Thinking chunk.
func TestMapChunk_Reasoning(t *testing.T) {
	t.Parallel()
	var c openaisdk.ChatCompletionChunk
	if err := json.Unmarshal([]byte(`{"choices":[{"delta":{"reasoning_content":"思考中"}}]}`), &c); err != nil {
		t.Fatalf("unmarshal chunk: %v", err)
	}
	chunk, emit := newStream(nil).mapChunk(c)
	if !emit || chunk.Token != "思考中" || !chunk.Thinking {
		t.Errorf("reasoning → %+v emit=%v; want Token=思考中 Thinking=true", chunk, emit)
	}
}

// TestToolAccumulate_CaptureOnFirst: id/name 仅首 delta 非空; arguments 累积.
func TestToolAccumulate_CaptureOnFirst(t *testing.T) {
	t.Parallel()
	s := newStream(nil)
	mk := func(idx int64, id, name, args string) openaisdk.ChatCompletionChunkChoiceDeltaToolCall {
		return openaisdk.ChatCompletionChunkChoiceDeltaToolCall{
			Index:    idx,
			ID:       id,
			Function: openaisdk.ChatCompletionChunkChoiceDeltaToolCallFunction{Name: name, Arguments: args},
		}
	}
	_ = s.accumulateToolCall(mk(0, "call_1", "topsql", ""))
	_ = s.accumulateToolCall(mk(0, "", "", `{"n":`))
	_ = s.accumulateToolCall(mk(0, "", "", ` 5}`))
	tools, err := s.decodeTools()
	if err != nil {
		t.Fatalf("decodeTools: %v", err)
	}
	if len(tools) != 1 || tools[0].ID != "call_1" || tools[0].Name != "topsql" {
		t.Fatalf("tool = %+v; want call_1/topsql", tools)
	}
	if _, ok := tools[0].Input["n"]; !ok {
		t.Errorf("tool input missing n: %+v", tools[0].Input)
	}
}

// TestDecodeTools_OrderByIndex: 乱序 index → 升序解码 (spec-1.21 executor 依赖).
func TestDecodeTools_OrderByIndex(t *testing.T) {
	t.Parallel()
	s := newStream(nil)
	for _, x := range []struct {
		idx  int64
		name string
	}{{2, "t2"}, {0, "t0"}, {1, "t1"}} {
		_ = s.accumulateToolCall(openaisdk.ChatCompletionChunkChoiceDeltaToolCall{
			Index: x.idx, ID: "i",
			Function: openaisdk.ChatCompletionChunkChoiceDeltaToolCallFunction{Name: x.name, Arguments: "{}"},
		})
	}
	tools, _ := s.decodeTools()
	for i, want := range []string{"t0", "t1", "t2"} {
		if tools[i].Name != want {
			t.Errorf("tools[%d]=%s; want %s (ascending index)", i, tools[i].Name, want)
		}
	}
}

func TestToolAccumulate_TooMany(t *testing.T) {
	t.Parallel()
	s := newStream(nil)
	var last error
	for i := int64(0); i <= int64(llm.MaxToolBlocks); i++ {
		last = s.accumulateToolCall(openaisdk.ChatCompletionChunkChoiceDeltaToolCall{
			Index: i, ID: "x",
			Function: openaisdk.ChatCompletionChunkChoiceDeltaToolCallFunction{Name: "n"},
		})
	}
	if !errors.Is(last, llm.ErrDecodeFailed) {
		t.Errorf("exceeding MaxToolBlocks should ErrDecodeFailed; got %v", last)
	}
}

func TestToParams_Tools(t *testing.T) {
	t.Parallel()
	p, _ := New(Config{APIKey: "sk", Model: "m", BaseURL: "http://x/v1"})
	req := llm.Request{
		Messages:  []llm.Message{{Role: llm.RoleUser, Content: []llm.ContentBlock{{Type: llm.BlockText, Text: "hi"}}}},
		MaxTokens: 100,
		Tools:     []llm.ToolSchema{{Name: "topsql", Description: "top sql", InputSchema: map[string]any{"type": "object"}}},
	}
	params := p.toParams(req)
	if len(params.Tools) != 1 || params.Tools[0].Function.Name != "topsql" {
		t.Errorf("Tools = %+v; want 1 topsql", params.Tools)
	}
}
