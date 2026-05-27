// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush
//
// openai.go — Provider construction + request mapping (package doc in doc.go).

package openai

import (
	"context"
	"strings"

	openaisdk "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	"github.com/sqlrush/opendbx/internal/domain/llm"
)

// Config configures the OpenAI-compat Provider (spec-1.20.1 D-1).
type Config struct {
	APIKey  string
	Model   string
	BaseURL string // 必填 (无默认; openai-compat 各厂商端点)
	// Name is the Provider.Name() value: "openai-compat" (默认) | "ollama".
	Name string
	// AllowKeyless permits an empty APIKey (ollama 本地无 key; cloud=false).
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

// New constructs an OpenAI-compat Provider. A missing APIKey returns
// LLM.AUTH_FAILED unless AllowKeyless (ollama). ambient OPENAI_API_KEY /
// OPENAI_BASE_URL are NOT used — key/url come only from config (spec-1.20
// config-only contract; spec-1.20.1 Q8/codex HIGH-8): we always pass
// explicit option.WithAPIKey / WithBaseURL.
func New(cfg Config) (*Provider, error) {
	if !cfg.AllowKeyless && cfg.APIKey == "" {
		return nil, llm.ErrAuthFailed
	}
	name := cfg.Name
	if name == "" {
		name = "openai-compat"
	}
	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
		option.WithBaseURL(cfg.BaseURL),
	}
	return &Provider{
		client: openaisdk.NewClient(opts...),
		model:  cfg.Model,
		name:   name,
	}, nil
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
// T-4 骨架: System (拼接 \n\n) + Messages (text) + Model + MaxCompletionTokens
// + Temperature. Tools / 多-block content / ThinkingBudget drop 细节 → T-5.
func (p *Provider) toParams(req llm.Request) openaisdk.ChatCompletionNewParams {
	msgs := make([]openaisdk.ChatCompletionMessageParamUnion, 0, len(req.Messages)+1)
	if sys := joinSystem(req.System); sys != "" {
		msgs = append(msgs, openaisdk.SystemMessage(sys))
	}
	for _, m := range req.Messages {
		text := firstText(m.Content) // T-5: 多-block / tool_use / tool_result 完整映射
		if m.Role == llm.RoleAssistant {
			msgs = append(msgs, openaisdk.AssistantMessage(text))
		} else {
			msgs = append(msgs, openaisdk.UserMessage(text))
		}
	}
	params := openaisdk.ChatCompletionNewParams{
		Model:               openaisdk.ChatModel(p.model),
		Messages:            msgs,
		MaxCompletionTokens: openaisdk.Int(int64(req.MaxTokens)),
	}
	// Temperature *float64 → opt (nil → default; 0.0 valid).
	if req.Temperature != nil {
		params.Temperature = openaisdk.Float(*req.Temperature)
	}
	// ThinkingBudget is dropped on the request side for openai (no OpenAI
	// equivalent; spec-1.20.1 Q3 provider-agnostic ValidateRequest still
	// checks the budget, adapter ignores it). Tools → T-5.
	return params
}

// joinSystem concatenates non-empty SystemBlock texts with "\n\n"
// (spec-1.20.1 Q2; CacheBreak ignored — OpenAI 自动 caching, 无 cache_control).
func joinSystem(blocks []llm.SystemBlock) string {
	parts := make([]string, 0, len(blocks))
	for _, b := range blocks {
		if b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n\n")
}

// firstText returns the first BlockText text (T-4 骨架; T-5 handles
// multi-block / BlockToolUse / BlockToolResult).
func firstText(blocks []llm.ContentBlock) string {
	for _, c := range blocks {
		if c.Type == llm.BlockText {
			return c.Text
		}
	}
	return ""
}
