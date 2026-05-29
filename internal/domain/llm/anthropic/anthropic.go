// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package anthropic

import (
	"context"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/param"

	"github.com/sqlrush/opendbx/internal/domain/llm"
)

// Config configures the Anthropic Provider. APIKey is required; BaseURL
// is optional (proxy / regional endpoint override, spec-3.9).
type Config struct {
	APIKey  string
	Model   string // e.g. "claude-sonnet-4-6"; config-driven (规则 16)
	BaseURL string // optional
}

// Provider implements llm.Provider over the Anthropic SDK.
type Provider struct {
	client anthropicsdk.Client
	model  string
}

// Compile-time interface satisfaction.
var _ llm.Provider = (*Provider)(nil)

// New constructs an Anthropic Provider. A missing APIKey returns
// LLM.AUTH_FAILED (原则 3: explicit, no fallback).
func New(cfg Config) (*Provider, error) {
	if cfg.APIKey == "" {
		return nil, llm.ErrAuthFailed
	}
	opts := []option.RequestOption{option.WithAPIKey(cfg.APIKey)}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	return &Provider{
		client: anthropicsdk.NewClient(opts...),
		model:  cfg.Model,
	}, nil
}

// Name implements llm.Provider.
func (p *Provider) Name() string { return "anthropic" }

// Stream implements llm.Provider. Validates the request, maps it to SDK
// params, and returns an llm.Stream wrapping the SDK SSE stream.
func (p *Provider) Stream(ctx context.Context, req llm.Request) (llm.Stream, error) {
	// errcode-lint:exempt -- spec-1.20 D-8: err is already a registered LLM.* errcode — ValidateRequest→REQUEST_INVALID, toParams→DECODE_FAILED/REQUEST_INVALID. Both returns pass it through; re-wrapping would shadow the code/hint.
	if err := llm.ValidateRequest(req); err != nil {
		return nil, err
	}
	params, err := p.toParams(req)
	if err != nil {
		return nil, err
	}
	sdkStream := p.client.Messages.NewStreaming(ctx, params)
	return newStream(sdkStream), nil
}

// toParams maps a model-agnostic llm.Request to SDK MessageNewParams.
func (p *Provider) toParams(req llm.Request) (anthropicsdk.MessageNewParams, error) {
	params := anthropicsdk.MessageNewParams{
		Model:     anthropicsdk.Model(p.model),
		MaxTokens: int64(req.MaxTokens),
	}

	// System blocks with per-block cache_control (R2 H-2).
	if len(req.System) > 0 {
		blocks := make([]anthropicsdk.TextBlockParam, 0, len(req.System))
		for _, sb := range req.System {
			tb := anthropicsdk.TextBlockParam{Text: sb.Text}
			if sb.CacheBreak {
				tb.CacheControl = anthropicsdk.NewCacheControlEphemeralParam()
			}
			blocks = append(blocks, tb)
		}
		params.System = blocks
	}

	// Messages (Role validated by ValidateRequest to user/assistant only).
	msgs, err := toMessages(req.Messages)
	if err != nil {
		return anthropicsdk.MessageNewParams{}, err
	}
	params.Messages = msgs

	// Temperature *float64 → param.Opt (0.0 is a valid value; R2 H-4).
	if req.Temperature != nil {
		params.Temperature = param.NewOpt(*req.Temperature)
	}

	// Thinking config (request-side; R2 H-3). Budget validated ≥1024 and
	// < MaxTokens by ValidateRequest.
	if req.ThinkingMode == llm.ThinkingEnabled {
		params.Thinking = anthropicsdk.ThinkingConfigParamOfEnabled(int64(req.ThinkingBudget))
	}

	// Tools (decode-only spec-1.20; D-4).
	if len(req.Tools) > 0 {
		params.Tools = toTools(req.Tools)
	}

	return params, nil
}

// toMessages maps llm.Message slice to SDK MessageParam slice (spec-1.21
// D-2: multi-turn round-trip). Exhaustive switch on BlockType with default
// → LLM.REQUEST_INVALID; an unknown BlockType MUST NOT panic on the IO
// path (规则 12). Per-block nil-guard enforces the spec-1.21 D-1 contract
// so a mis-tagged ContentBlock surfaces as a clean errcode rather than a
// nil-deref crash deep in the SDK.
//
// CC baseline B-63 (codex VERIFIED, anthropic-sdk-go v1.45.0):
//   - NewToolUseBlock(id string, input any, name string)
//   - NewToolResultBlock(toolUseID string, content string, isError bool)
func toMessages(msgs []llm.Message) ([]anthropicsdk.MessageParam, error) {
	out := make([]anthropicsdk.MessageParam, 0, len(msgs))
	for _, m := range msgs {
		blocks := make([]anthropicsdk.ContentBlockParamUnion, 0, len(m.Content))
		for _, c := range m.Content {
			switch c.Type {
			case llm.BlockText:
				blocks = append(blocks, anthropicsdk.NewTextBlock(c.Text))
			case llm.BlockToolUse:
				if c.ToolUse == nil {
					return nil, llm.RequestInvalidf("BlockToolUse with nil ToolUse (spec-1.21 D-1 nil-guard)")
				}
				// SDK accepts `Input any` directly; map[string]any from
				// spec-1.20 DecodeToolInput is a valid value (codex T-2
				// VERIFIED). Order id/input/name matches SDK signature.
				blocks = append(blocks, anthropicsdk.NewToolUseBlock(c.ToolUse.ID, c.ToolUse.Input, c.ToolUse.Name))
			case llm.BlockToolResult:
				if c.ToolResult == nil {
					return nil, llm.RequestInvalidf("BlockToolResult with nil ToolResult (spec-1.21 D-1 nil-guard)")
				}
				blocks = append(blocks, anthropicsdk.NewToolResultBlock(c.ToolResult.ToolUseID, c.ToolResult.Content, c.ToolResult.IsError))
			default:
				return nil, llm.RequestInvalidf("unhandled content BlockType (append-only contract requires explicit case)")
			}
		}
		if m.Role == llm.RoleAssistant {
			out = append(out, anthropicsdk.NewAssistantMessage(blocks...))
		} else {
			out = append(out, anthropicsdk.NewUserMessage(blocks...))
		}
	}
	return out, nil
}

// toTools maps llm.ToolSchema slice to SDK tool union params (D-4 / R2 MED-1).
func toTools(tools []llm.ToolSchema) []anthropicsdk.ToolUnionParam {
	out := make([]anthropicsdk.ToolUnionParam, 0, len(tools))
	for _, t := range tools {
		schema := anthropicsdk.ToolInputSchemaParam{}
		if props, ok := t.InputSchema["properties"]; ok {
			schema.Properties = props
		}
		if reqd, ok := t.InputSchema["required"].([]string); ok {
			schema.Required = reqd
		}
		tool := anthropicsdk.ToolParam{
			Name:        t.Name,
			InputSchema: schema,
		}
		if t.Description != "" {
			tool.Description = param.NewOpt(t.Description)
		}
		out = append(out, anthropicsdk.ToolUnionParam{OfTool: &tool})
	}
	return out
}
