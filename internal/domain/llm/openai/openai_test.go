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
		// tool_calls with no accumulated tool deltas → Error (R-fix codex HIGH-3:
		// protocol anomaly, do not emit silent FinishToolUse{ToolUses:nil}).
		{"tool_calls", llm.FinishError, llm.ErrDecodeFailed},
		// function_call (legacy Functions API) is rejected explicitly (R-fix).
		{"function_call", llm.FinishError, llm.ErrDecodeFailed},
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

// TestClassifyOpenAIErr: SDK API error status → registered LLM.*; non-SDK
// errors pass through (mirror anthropic classifyStreamErr).
func TestClassifyOpenAIErr(t *testing.T) {
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
		{404, llm.ErrUnavailable},
		{429, llm.ErrUnavailable},
		{500, llm.ErrUnavailable},
	}
	for _, tc := range cases {
		got := classifyOpenAIErr(&openaisdk.Error{StatusCode: tc.status})
		if !errors.Is(got, tc.want) {
			t.Errorf("status %d → %v; want %v", tc.status, got, tc.want)
		}
	}
	if classifyOpenAIErr(nil) != nil {
		t.Errorf("nil → nil")
	}
	passthrough := errors.New("ctx boom")
	if got := classifyOpenAIErr(passthrough); !errors.Is(got, passthrough) {
		t.Errorf("non-SDK error should pass through; got %v", got)
	}
	if !errors.Is(classifyOpenAIErr(llm.ErrDecodeFailed), llm.ErrDecodeFailed) {
		t.Errorf("ErrDecodeFailed should pass through")
	}
}

// TestNew_MissingBaseURL: cloud config without BaseURL must NOT fall back to
// the SDK default api.openai.com — would silently mis-route deepseek/qwen
// (R-fix claude HIGH-1 / codex MED-1).
func TestNew_MissingBaseURL(t *testing.T) {
	t.Parallel()
	_, err := New(Config{APIKey: "sk", Model: "m"})
	if !errors.Is(err, llm.ErrRequestInvalid) {
		t.Errorf("missing BaseURL → %v; want ErrRequestInvalid", err)
	}
}

// TestNew_KeylessNonLoopback: AllowKeyless on a non-loopback BaseURL must be
// rejected — otherwise a cloud endpoint could bypass the API-key requirement
// by setting AllowKeyless=true (R-fix codex MED-2).
func TestNew_KeylessNonLoopback(t *testing.T) {
	t.Parallel()
	cases := []string{
		"https://api.deepseek.com/v1",
		"http://10.0.0.5:11434/v1",
		"http://example.com/v1",
	}
	for _, u := range cases {
		_, err := New(Config{Model: "m", BaseURL: u, AllowKeyless: true})
		if !errors.Is(err, llm.ErrRequestInvalid) {
			t.Errorf("keyless+%s → %v; want ErrRequestInvalid", u, err)
		}
	}
}

// TestNew_KeylessLoopbackVariants: each accepted loopback host MUST construct.
func TestNew_KeylessLoopbackVariants(t *testing.T) {
	t.Parallel()
	for _, u := range []string{
		"http://localhost:11434/v1",
		"http://127.0.0.1:11434/v1",
		"http://[::1]:11434/v1",
	} {
		if _, err := New(Config{Model: "m", BaseURL: u, AllowKeyless: true}); err != nil {
			t.Errorf("keyless loopback %s → %v; want ok", u, err)
		}
	}
}

// TestMapChunk_Refusal: delta.refusal must surface as FinishRefusal +
// ErrProviderRefusal (distinct from content_filter platform-side block).
func TestMapChunk_Refusal(t *testing.T) {
	t.Parallel()
	c := openaisdk.ChatCompletionChunk{Choices: []openaisdk.ChatCompletionChunkChoice{
		{Delta: openaisdk.ChatCompletionChunkChoiceDelta{Refusal: "I cannot help with that."}},
	}}
	chunk, emit := newStream(nil).mapChunk(c)
	if !emit || chunk.FinishReason != llm.FinishRefusal || !errors.Is(chunk.Err, llm.ErrProviderRefusal) {
		t.Errorf("refusal → %+v emit=%v; want FinishRefusal+ErrProviderRefusal", chunk, emit)
	}
}

// TestMapChunk_ReasoningOversize: raw reasoning_content payload exceeding the
// 64 KB bound must be dropped to "" (prevents per-delta huge allocation).
func TestMapChunk_ReasoningOversize(t *testing.T) {
	t.Parallel()
	// Build a JSON-encoded string of len > 64 KB.
	huge := make([]byte, maxReasoningChunkBytes+10)
	for i := range huge {
		huge[i] = 'x'
	}
	payload, _ := json.Marshal(string(huge))
	chunkJSON := []byte(`{"choices":[{"delta":{"reasoning_content":` + string(payload) + `}}]}`)
	var c openaisdk.ChatCompletionChunk
	if err := json.Unmarshal(chunkJSON, &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	_, emit := newStream(nil).mapChunk(c)
	if emit {
		t.Errorf("oversize reasoning_content should be dropped (no emit)")
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

// TestToParams_MaxTokensLegacy guards the R-fix codex HIGH-1 decision:
// emit `max_tokens` (legacy) rather than `max_completion_tokens` so
// DeepSeek / Qwen / Ollama and other openai-compat endpoints (which often
// implement only the legacy parameter) work without manual override.
func TestToParams_MaxTokensLegacy(t *testing.T) {
	t.Parallel()
	p, _ := New(Config{APIKey: "sk", Model: "m", BaseURL: "http://x/v1"})
	req := llm.Request{
		Messages:  []llm.Message{{Role: llm.RoleUser, Content: []llm.ContentBlock{{Type: llm.BlockText, Text: "hi"}}}},
		MaxTokens: 256,
	}
	params := p.toParams(req)
	if params.MaxTokens.Or(-1) != 256 {
		t.Errorf("MaxTokens = %v; want 256 (legacy max_tokens, R-fix HIGH-1)", params.MaxTokens)
	}
	if params.MaxCompletionTokens.Or(-1) != -1 {
		t.Errorf("MaxCompletionTokens MUST stay unset for openai-compat breadth (R-fix HIGH-1); got %v", params.MaxCompletionTokens)
	}
}
