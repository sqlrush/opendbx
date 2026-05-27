// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package anthropic adapts github.com/anthropics/anthropic-sdk-go
// (pinned v1.45.0) to the model-agnostic llm.Provider (spec-1.20 D-2).
//
// This is one of the only two packages permitted to import a vendor LLM
// SDK (IMP-7 isolation: anthropic/ + openai/). All SDK-type conversion
// lives here; the llm.Provider interface keeps app-layer code SDK-free
// (规则 16).
//
// Mapping (real SDK shapes, codex-verified):
//   - Request.System []SystemBlock → []TextBlockParam with per-block
//     cache_control: ephemeral (spec-1.20 R2 H-2 / minimal prompt cache)
//   - Request.Temperature *float64 → param.Opt[float64] (0.0 is valid)
//   - Request.ThinkingMode/Budget → ThinkingConfigParamUnion (enabled
//     budget counts toward max_tokens; R2 H-3)
//   - Request.Tools → []ToolUnionParam (ToolInputSchemaParam)
//   - SSE event stream → llm.Stream iterator; text_delta → Token,
//     thinking_delta → {Token,Thinking}, input_json_delta accumulated per
//     block index then decoded (JSON guard, D-4), stop_reason → full
//     FinishReason enum (R2 H-6), citations/signature_delta ignored (❌-14)
//   - ctx cancel → Close + Canceled/Deadline classified by the caller's
//     finishFromErr (R2 MED)
package anthropic
