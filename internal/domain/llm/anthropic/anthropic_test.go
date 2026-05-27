// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package anthropic

import (
	"errors"
	"strings"
	"testing"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"

	"github.com/sqlrush/opendbx/internal/domain/llm"
)

func TestNew_MissingKey(t *testing.T) {
	t.Parallel()
	_, err := New(Config{Model: "claude-sonnet-4-6"})
	if !errors.Is(err, llm.ErrAuthFailed) {
		t.Errorf("New without APIKey err = %v; want ErrAuthFailed", err)
	}
}

func TestNew_OK(t *testing.T) {
	t.Parallel()
	p, err := New(Config{APIKey: "sk-test", Model: "claude-sonnet-4-6"})
	if err != nil || p == nil {
		t.Fatalf("New: %v", err)
	}
	if p.Name() != "anthropic" {
		t.Errorf("Name = %q; want anthropic", p.Name())
	}
}

func TestStream_ValidatesRequest(t *testing.T) {
	t.Parallel()
	p, _ := New(Config{APIKey: "sk-test", Model: "m"})
	_, err := p.Stream(t.Context(), llm.Request{}) // empty messages
	if !errors.Is(err, llm.ErrRequestInvalid) {
		t.Errorf("Stream invalid req err = %v; want ErrRequestInvalid", err)
	}
}

func TestMapStopReason(t *testing.T) {
	t.Parallel()
	cases := []struct {
		sr   anthropicsdk.StopReason
		want llm.FinishReason
	}{
		{anthropicsdk.StopReasonEndTurn, llm.FinishStop},
		{anthropicsdk.StopReasonMaxTokens, llm.FinishLength},
		{anthropicsdk.StopReasonToolUse, llm.FinishToolUse},
		{anthropicsdk.StopReasonStopSequence, llm.FinishStopSequence},
		{anthropicsdk.StopReasonPauseTurn, llm.FinishPause},
		{anthropicsdk.StopReasonRefusal, llm.FinishRefusal},
		{anthropicsdk.StopReason(""), llm.FinishUnset},
		{anthropicsdk.StopReason("future_unknown"), llm.FinishStop},
	}
	for _, tc := range cases {
		if got := mapStopReason(tc.sr); got != tc.want {
			t.Errorf("mapStopReason(%q) = %v; want %v", tc.sr, got, tc.want)
		}
	}
}

func TestDecodeToolInput(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{"empty → empty object", "", false},
		{"object", `{"n": 5, "q": "select"}`, false},
		{"nested object", `{"a": {"b": {"c": 1}}}`, false},
		{"non-object array", `[1,2,3]`, true},
		{"non-object scalar", `42`, true},
		{"malformed", `{"a":`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := decodeToolInput([]byte(tc.raw))
			if tc.wantErr != (err != nil) {
				t.Errorf("decodeToolInput(%q) err=%v; wantErr=%v", tc.raw, err, tc.wantErr)
			}
			if err != nil && !errors.Is(err, llm.ErrDecodeFailed) {
				t.Errorf("err %v should be ErrDecodeFailed", err)
			}
		})
	}
}

func TestDecodeToolInput_Oversize(t *testing.T) {
	t.Parallel()
	big := `{"x":"` + strings.Repeat("a", maxToolInputBytes) + `"}`
	if _, err := decodeToolInput([]byte(big)); !errors.Is(err, llm.ErrDecodeFailed) {
		t.Errorf("oversize input should be ErrDecodeFailed; got %v", err)
	}
}

func TestDecodeToolInput_TooDeep(t *testing.T) {
	t.Parallel()
	deep := strings.Repeat(`{"a":`, maxToolInputDepth+2) + "1" + strings.Repeat("}", maxToolInputDepth+2)
	if _, err := decodeToolInput([]byte(deep)); !errors.Is(err, llm.ErrDecodeFailed) {
		t.Errorf("too-deep input should be ErrDecodeFailed; got %v", err)
	}
}

func TestToParams_Mapping(t *testing.T) {
	t.Parallel()
	p, _ := New(Config{APIKey: "sk", Model: "claude-sonnet-4-6"})
	temp := 0.0 // valid 0 (R2 H-4)
	req := llm.Request{
		System: []llm.SystemBlock{
			{Text: "base prompt", CacheBreak: true},
			{Text: "no cache"},
		},
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: []llm.ContentBlock{{Type: llm.BlockText, Text: "hi"}}},
			{Role: llm.RoleAssistant, Content: []llm.ContentBlock{{Type: llm.BlockText, Text: "hello"}}},
		},
		Tools:          []llm.ToolSchema{{Name: "topsql", Description: "top SQL", InputSchema: map[string]any{"properties": map[string]any{}, "required": []string{"n"}}}},
		MaxTokens:      1024,
		Temperature:    &temp,
		ThinkingMode:   llm.ThinkingEnabled,
		ThinkingBudget: 512 + 512, // 1024
	}
	params, err := p.toParams(req)
	if err != nil {
		t.Fatalf("toParams: %v", err)
	}
	if params.MaxTokens != 1024 {
		t.Errorf("MaxTokens = %d; want 1024", params.MaxTokens)
	}
	if len(params.System) != 2 || params.System[0].Text != "base prompt" {
		t.Errorf("System mapping wrong: %+v", params.System)
	}
	// cache_control set on block 0 (CacheBreak), not block 1.
	if params.System[0].CacheControl.Type == "" {
		t.Errorf("System[0] cache_control not set despite CacheBreak")
	}
	if params.System[1].CacheControl.Type != "" {
		t.Errorf("System[1] cache_control set without CacheBreak")
	}
	if !params.Temperature.Valid() || params.Temperature.Value != 0.0 {
		t.Errorf("Temperature opt = %+v; want valid 0.0", params.Temperature)
	}
	if len(params.Messages) != 2 {
		t.Errorf("Messages len = %d; want 2", len(params.Messages))
	}
	if len(params.Tools) != 1 {
		t.Errorf("Tools len = %d; want 1", len(params.Tools))
	}
}

func TestToParams_NoTemperature(t *testing.T) {
	t.Parallel()
	p, _ := New(Config{APIKey: "sk", Model: "m"})
	req := llm.Request{
		Messages:  []llm.Message{{Role: llm.RoleUser, Content: []llm.ContentBlock{{Type: llm.BlockText, Text: "x"}}}},
		MaxTokens: 100,
	}
	params, _ := p.toParams(req)
	if params.Temperature.Valid() {
		t.Errorf("Temperature should be unset (nil → provider default); got valid %v", params.Temperature.Value)
	}
}

// --- mapEvent via SDK event JSON (SDK test pattern: UnmarshalJSON) ---

func event(t *testing.T, jsonStr string) anthropicsdk.MessageStreamEventUnion {
	t.Helper()
	var ev anthropicsdk.MessageStreamEventUnion
	if err := (&ev).UnmarshalJSON([]byte(jsonStr)); err != nil {
		t.Fatalf("UnmarshalJSON(%s): %v", jsonStr, err)
	}
	return ev
}

func TestMapEvent_TextDelta(t *testing.T) {
	t.Parallel()
	s := newStream(nil)
	c, emit := s.mapEvent(event(t, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`))
	if !emit || c.Token != "hello" || c.Thinking {
		t.Errorf("text_delta → %+v emit=%v; want Token=hello", c, emit)
	}
}

func TestMapEvent_ThinkingDelta(t *testing.T) {
	t.Parallel()
	s := newStream(nil)
	c, emit := s.mapEvent(event(t, `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"reasoning"}}`))
	if !emit || c.Token != "reasoning" || !c.Thinking {
		t.Errorf("thinking_delta → %+v emit=%v; want Token=reasoning Thinking=true", c, emit)
	}
}

func TestMapEvent_IgnoredDeltas(t *testing.T) {
	t.Parallel()
	s := newStream(nil)
	for _, j := range []string{
		`{"type":"message_start","message":{}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_stop"}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig"}}`,
	} {
		if _, emit := s.mapEvent(event(t, j)); emit {
			t.Errorf("event %s should not emit a chunk", j)
		}
	}
}

func TestMapEvent_ToolUseAccumulateAndDecode(t *testing.T) {
	t.Parallel()
	s := newStream(nil)
	// tool_use block start + input_json_delta accumulation + stop_reason.
	s.mapEvent(event(t, `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"topsql","input":{}}}`))
	s.mapEvent(event(t, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"n\":"}}`))
	s.mapEvent(event(t, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":" 5}"}}`))
	c, emit := s.mapEvent(event(t, `{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{}}`))
	if !emit || c.FinishReason != llm.FinishToolUse {
		t.Fatalf("message_delta tool_use → %+v emit=%v", c, emit)
	}
	if len(c.ToolUses) != 1 || c.ToolUses[0].Name != "topsql" || c.ToolUses[0].ID != "toolu_1" {
		t.Fatalf("decoded ToolUses = %+v; want [topsql/toolu_1]", c.ToolUses)
	}
	if n, _ := c.ToolUses[0].Input["n"].(interface{ Int64() (int64, error) }); n == nil {
		// UseNumber → json.Number; just assert presence.
		if _, ok := c.ToolUses[0].Input["n"]; !ok {
			t.Errorf("tool input missing key n: %+v", c.ToolUses[0].Input)
		}
	}
}

func TestMapEvent_StopReasonLength(t *testing.T) {
	t.Parallel()
	s := newStream(nil)
	c, emit := s.mapEvent(event(t, `{"type":"message_delta","delta":{"stop_reason":"max_tokens"},"usage":{}}`))
	if !emit || c.FinishReason != llm.FinishLength {
		t.Errorf("max_tokens → %+v; want FinishLength", c)
	}
}

// BenchmarkSSEMap targets spec-1.20 § 4.4 BenchmarkSSEMap < 500 ns/op
// (one text_delta event → llm.Chunk via mapEvent).
func BenchmarkSSEMap(b *testing.B) {
	var ev anthropicsdk.MessageStreamEventUnion
	_ = (&ev).UnmarshalJSON([]byte(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`))
	s := newStream(nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = s.mapEvent(ev)
	}
}
