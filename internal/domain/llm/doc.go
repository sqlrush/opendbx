// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package llm is the model-agnostic LLM gateway (spec-1.20 D-1; 规则 16).
// All LLM calls in opendbx go through the Provider interface; app-layer
// packages MUST NOT import any vendor SDK (IMP-7 tcell/LLM-SDK isolation
// allows SDK imports only in internal/domain/llm/anthropic and /openai).
//
// Layering invariants (spec-1.20 R2 H-2/H-3):
//   - llm imports zero vendor SDK and zero app/render package — it is a
//     pure domain contract consumed by app/cli/llmapp.
//   - Request / Chunk / ToolUse / SystemBlock / FinishReason are
//     opendbx-owned types; SDK-type conversion lives only in the adapter.
//
// Evolution contract (spec-1.20 R2 H-6; §3.7 类比) — 8 downstream specs
// (1.21 / 2.10 / 2.11 / 3.8 / 3.9 / 3.10 / 3.11) depend on this contract:
//   - Request / Chunk / ContentBlock are additive-only (new fields, never
//     rename/delete). BlockType switches MUST be exhaustive + default panic.
//   - tier auto-degrade (spec-3.11) is a NewProvider factory decorator
//     layer — NOT inside Stream. Stream stays single-provider.
//
// No-degradation invariant (原则 3 + AD-004 + R2 H-7): an unavailable /
// unauthenticated provider returns a registered LLM.* errcode — NEVER a
// rule-based fallback. Orchestration layers (spec-1.21 loop / factory
// degrade) may only switch BETWEEN LLM providers (tier degrade is still
// LLM); no non-LLM conclusion path is permitted.
package llm
