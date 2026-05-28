// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush
//
// openai.go — Provider construction + request mapping (package doc in doc.go).

package openai

import (
	"context"
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
	sdkStream := p.client.Chat.Completions.NewStreaming(ctx, p.toParams(req))
	return newStream(sdkStream), nil
}

// toParams maps a model-agnostic llm.Request to SDK ChatCompletionNewParams.
// Uses the legacy `max_tokens` (not `max_completion_tokens`) for broadest
// OpenAI-compat endpoint support: DeepSeek/Qwen/Ollama and other community
// endpoints implement the legacy parameter; max_completion_tokens is OpenAI's
// newer name and not universally supported (codex R-fix HIGH-1).
func (p *Provider) toParams(req llm.Request) openaisdk.ChatCompletionNewParams {
	msgs := make([]openaisdk.ChatCompletionMessageParamUnion, 0, len(req.Messages)+1)
	if sys := joinSystem(req.System); sys != "" {
		msgs = append(msgs, openaisdk.SystemMessage(sys))
	}
	for _, m := range req.Messages {
		// 1.20.1 single-turn: text-only content. Multi-block (BlockToolUse /
		// BlockToolResult) round-trip is spec-1.21.
		text := firstText(m.Content)
		if m.Role == llm.RoleAssistant {
			msgs = append(msgs, openaisdk.AssistantMessage(text))
		} else {
			msgs = append(msgs, openaisdk.UserMessage(text))
		}
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
	return params
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

// firstText returns the first BlockText text. Multi-block / BlockToolUse /
// BlockToolResult round-trip mapping is spec-1.21 (two adapters together).
func firstText(blocks []llm.ContentBlock) string {
	for _, c := range blocks {
		if c.Type == llm.BlockText {
			return c.Text
		}
	}
	return ""
}
