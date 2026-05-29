// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush
//
// openai.go — Provider construction + request mapping (package doc in doc.go).

package openai

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"

	openaisdk "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"

	"github.com/sqlrush/opendbx/internal/domain/llm"
)

// Config configures the OpenAI-compat Provider (spec-1.20.1 D-1).
type Config struct {
	APIKey  string
	Model   string
	BaseURL string // required (no default); each OpenAI-compat vendor has its own endpoint
	// Name is the Provider.Name() value: "openai-compat" (default) | "ollama".
	Name string
	// AllowKeyless permits an empty APIKey AND requires BaseURL to be a
	// loopback host (ollama local); cloud openai-compat MUST have a key.
	AllowKeyless bool
}

// Provider implements llm.Provider over the openai-go SDK.
type Provider struct {
	client openaisdk.Client
	model  string
	name   string
}

// Compile-time interface satisfaction.
var _ llm.Provider = (*Provider)(nil)

// New constructs an OpenAI-compat Provider.
//
// Config enforcement (spec-1.20.1 R-fix absorb):
//   - !AllowKeyless && APIKey=="" → LLM.AUTH_FAILED (cloud key required).
//   - BaseURL=="" → LLM.REQUEST_INVALID (no SDK default fallback to
//     api.openai.com, which would silently route deepseek/qwen mis-config to
//     OpenAI — claude HIGH-1 / codex MED).
//   - AllowKeyless==true and BaseURL host NOT loopback → LLM.REQUEST_INVALID
//     (codex MED: ollama keyless must be local; prevents bypassing the cloud
//     key requirement by setting AllowKeyless on a cloud endpoint).
//
// Ambient env isolation (spec-1.20.1 Q8 / codex HIGH-2): the openai-go SDK
// reads 5 env vars in DefaultClientOptions — OPENAI_API_KEY, OPENAI_BASE_URL,
// OPENAI_ORG_ID, OPENAI_PROJECT_ID, OPENAI_WEBHOOK_SECRET. We override
// API_KEY/BASE_URL with explicit options AND suppress the
// OpenAI-Organization / OpenAI-Project headers (set from ORG_ID/PROJECT_ID)
// via option.WithHeaderDel — preserving the spec-1.20 config-only contract.
// WEBHOOK_SECRET is unused on the chat completions path.
func New(cfg Config) (*Provider, error) {
	if !cfg.AllowKeyless && cfg.APIKey == "" {
		return nil, llm.ErrAuthFailed
	}
	if cfg.BaseURL == "" {
		return nil, llm.ErrRequestInvalid
	}
	if cfg.AllowKeyless {
		u, err := url.Parse(cfg.BaseURL)
		if err != nil || !isLoopbackHost(u.Hostname()) {
			return nil, llm.ErrRequestInvalid // keyless permitted only on loopback
		}
	}
	name := cfg.Name
	if name == "" {
		name = "openai-compat"
	}
	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
		option.WithBaseURL(cfg.BaseURL),
		// Suppress ambient OPENAI_ORG_ID / OPENAI_PROJECT_ID header injection.
		option.WithHeaderDel("OpenAI-Organization"),
		option.WithHeaderDel("OpenAI-Project"),
	}
	return &Provider{
		client: openaisdk.NewClient(opts...),
		model:  cfg.Model,
		name:   name,
	}, nil
}

// isLoopbackHost reports whether host is a loopback address that an ollama
// (or other local OpenAI-compat) endpoint can legitimately use without a key.
func isLoopbackHost(host string) bool {
	switch host {
	case "localhost", "127.0.0.1", "::1", "0.0.0.0":
		return true
	}
	return false
}

// Name implements llm.Provider ("openai-compat" | "ollama").
func (p *Provider) Name() string { return p.name }

// Stream implements llm.Provider. Validates the request, maps it to SDK
// params, and returns an llm.Stream wrapping the SDK SSE stream.
func (p *Provider) Stream(ctx context.Context, req llm.Request) (llm.Stream, error) {
	// errcode-lint:exempt -- spec-1.20.1 D-1: err is a registered LLM.* errcode (ValidateRequest→REQUEST_INVALID); pass-through, mirrors anthropic.Stream.
	if err := llm.ValidateRequest(req); err != nil {
		return nil, err
	}
	params, err := p.toParams(req)
	if err != nil {
		return nil, err
	}
	sdkStream := p.client.Chat.Completions.NewStreaming(ctx, params)
	return newStream(sdkStream), nil
}

// toParams maps a model-agnostic llm.Request to SDK ChatCompletionNewParams.
// Uses the legacy `max_tokens` (not `max_completion_tokens`) for broadest
// OpenAI-compat endpoint support: DeepSeek/Qwen/Ollama and other community
// endpoints implement the legacy parameter; max_completion_tokens is OpenAI's
// newer name and not universally supported (codex R-fix HIGH-1).
//
// Multi-turn round-trip (spec-1.21 D-2): assistant BlockToolUse → SDK
// ToolCalls on the AssistantMessage; user BlockToolResult → distinct
// role=tool ChatCompletionMessageParamUnion entries (OpenAI protocol
// surfaces tool_result as its own message role, NOT as a content block
// inside the user turn — this is a fundamental shape difference from
// Anthropic that the adapter normalises here).
func (p *Provider) toParams(req llm.Request) (openaisdk.ChatCompletionNewParams, error) {
	msgs := make([]openaisdk.ChatCompletionMessageParamUnion, 0, len(req.Messages)+1)
	if sys := joinSystem(req.System); sys != "" {
		msgs = append(msgs, openaisdk.SystemMessage(sys))
	}
	for _, m := range req.Messages {
		expanded, err := messageToSDK(m)
		if err != nil {
			return openaisdk.ChatCompletionNewParams{}, err
		}
		msgs = append(msgs, expanded...)
	}
	params := openaisdk.ChatCompletionNewParams{
		Model:     openaisdk.ChatModel(p.model),
		Messages:  msgs,
		MaxTokens: openaisdk.Int(int64(req.MaxTokens)), // legacy, broad compat
	}
	// Temperature *float64 → opt (nil → default; 0.0 valid).
	if req.Temperature != nil {
		params.Temperature = openaisdk.Float(*req.Temperature)
	}
	// Tools → OpenAI function tools (Strict omitted — constrained-gen → spec-3.11;
	// decode-only in 1.20.1, multi-turn tool_result round-trip → spec-1.21).
	if len(req.Tools) > 0 {
		tools := make([]openaisdk.ChatCompletionToolParam, 0, len(req.Tools))
		for _, t := range req.Tools {
			fn := shared.FunctionDefinitionParam{Name: t.Name}
			if t.Description != "" {
				fn.Description = openaisdk.String(t.Description)
			}
			if t.InputSchema != nil {
				fn.Parameters = shared.FunctionParameters(t.InputSchema)
			}
			tools = append(tools, openaisdk.ChatCompletionToolParam{Function: fn})
		}
		params.Tools = tools
	}
	// ThinkingBudget is dropped on the request side for openai (no OpenAI
	// equivalent; spec-1.20.1 Q3 keeps the provider-agnostic ValidateRequest
	// budget check, adapter ignores the value).
	return params, nil
}

// messageToSDK expands one llm.Message into 1..N SDK message-param entries.
//
// Shape rules (spec-1.21 D-2):
//
//   - assistant turn — exactly one AssistantMessage carrying concatenated
//     BlockText content and BlockToolUse entries collected as
//     ChatCompletionMessageToolCallParam{ID, Function:{Name, Arguments}}.
//     Arguments must be a JSON string (SDK contract); we marshal
//     ToolUse.Input map[string]any here (nil map → "{}" — OpenAI's required
//     no-arg shape, distinct from Anthropic's nil/absent input).
//   - user turn — each BlockToolResult becomes ONE distinct role=tool
//     ChatCompletionMessageParamUnion (OpenAI does not nest tool_result
//     inside a user content block). Any BlockText payload in the same
//     user turn is appended afterwards as a separate UserMessage so the
//     adapter never silently merges user prose into a tool message.
//   - both turns reject BlockToolResult on assistant role and BlockToolUse
//     on user role with LLM.REQUEST_INVALID (defensive: well-formed Loop
//     output never mixes these, but a misuse must not silently round-trip).
//   - nil-guard per spec-1.21 D-1: missing ToolUse / ToolResult pointer
//     surfaces as LLM.REQUEST_INVALID rather than a nil-deref later.
func messageToSDK(m llm.Message) ([]openaisdk.ChatCompletionMessageParamUnion, error) {
	switch m.Role {
	case llm.RoleAssistant:
		return assistantToSDK(m.Content)
	default:
		return userToSDK(m.Content)
	}
}

func assistantToSDK(blocks []llm.ContentBlock) ([]openaisdk.ChatCompletionMessageParamUnion, error) {
	var text strings.Builder
	var toolCalls []openaisdk.ChatCompletionMessageToolCallParam
	for _, c := range blocks {
		switch c.Type {
		case llm.BlockText:
			text.WriteString(c.Text)
		case llm.BlockToolUse:
			if c.ToolUse == nil {
				return nil, llm.RequestInvalidf("BlockToolUse with nil ToolUse (spec-1.21 D-1 nil-guard)")
			}
			args, err := marshalToolInput(c.ToolUse.Input)
			if err != nil {
				return nil, llm.RequestInvalidf("ToolUse.Input not JSON-encodable: " + err.Error())
			}
			toolCalls = append(toolCalls, openaisdk.ChatCompletionMessageToolCallParam{
				ID: c.ToolUse.ID,
				Function: openaisdk.ChatCompletionMessageToolCallFunctionParam{
					Name:      c.ToolUse.Name,
					Arguments: args,
				},
			})
		case llm.BlockToolResult:
			return nil, llm.RequestInvalidf("BlockToolResult forbidden on assistant turn (tool_result is a user/tool-role payload)")
		default:
			return nil, llm.RequestInvalidf("unhandled content BlockType on assistant turn (append-only contract)")
		}
	}
	assistant := openaisdk.ChatCompletionAssistantMessageParam{}
	if text.Len() > 0 {
		assistant.Content.OfString = openaisdk.String(text.String())
	}
	if len(toolCalls) > 0 {
		assistant.ToolCalls = toolCalls
	}
	return []openaisdk.ChatCompletionMessageParamUnion{{OfAssistant: &assistant}}, nil
}

func userToSDK(blocks []llm.ContentBlock) ([]openaisdk.ChatCompletionMessageParamUnion, error) {
	out := make([]openaisdk.ChatCompletionMessageParamUnion, 0, len(blocks))
	var text strings.Builder
	for _, c := range blocks {
		switch c.Type {
		case llm.BlockText:
			text.WriteString(c.Text)
		case llm.BlockToolResult:
			if c.ToolResult == nil {
				return nil, llm.RequestInvalidf("BlockToolResult with nil ToolResult (spec-1.21 D-1 nil-guard)")
			}
			content := c.ToolResult.Content
			if c.ToolResult.IsError {
				// OpenAI has no is_error field on tool messages; prefix the
				// content so the model still sees the self-correctable
				// failure signal (spec-1.21 D-1 IsError contract preserved
				// across providers, not silently dropped).
				content = "[tool error] " + content
			}
			out = append(out, openaisdk.ToolMessage(content, c.ToolResult.ToolUseID))
		case llm.BlockToolUse:
			return nil, llm.RequestInvalidf("BlockToolUse forbidden on user turn (tool_use is an assistant-role payload)")
		default:
			return nil, llm.RequestInvalidf("unhandled content BlockType on user turn (append-only contract)")
		}
	}
	if text.Len() > 0 {
		out = append(out, openaisdk.UserMessage(text.String()))
	}
	if len(out) == 0 {
		// Empty user turn — still emit one UserMessage with empty content
		// so the conversation alternates and SDK does not reject an empty
		// messages slot. ValidateRequest already rejects an empty Messages
		// slice upstream.
		out = append(out, openaisdk.UserMessage(""))
	}
	return out, nil
}

// marshalToolInput renders ToolUse.Input as the JSON argument string
// expected by ChatCompletionMessageToolCallFunctionParam.Arguments.
// A nil map round-trips as "{}" (OpenAI's required empty-args shape).
func marshalToolInput(input map[string]any) (string, error) {
	if input == nil {
		return "{}", nil
	}
	b, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// joinSystem concatenates non-empty SystemBlock texts with "\n\n"
// (spec-1.20.1 Q2; CacheBreak ignored — OpenAI uses automatic prompt caching
// with no breakpoint primitive).
func joinSystem(blocks []llm.SystemBlock) string {
	parts := make([]string, 0, len(blocks))
	for _, b := range blocks {
		if b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n\n")
}

// (spec-1.20.1's `firstText` single-turn helper was retired in spec-1.21:
// messageToSDK now collects ALL BlockText blocks of a turn into a single
// concatenated content payload, alongside ToolCalls/ToolMessage expansion.
// Single-turn behaviour is preserved because a 1-block llm.Message round-
// trips the same way through assistantToSDK / userToSDK.)
