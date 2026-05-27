// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package llm

import "context"

// Role tags a Message author. spec-1.20 R2.1: system prompt is carried
// separately as Request.System []SystemBlock — Message.Role is only
// User/Assistant. There is no RoleSystem constant; a Request whose
// Messages contain a "system" role (from any source) is rejected with
// LLM.REQUEST_INVALID (ValidateRequest).
type Role string

// Role values (system prompt is separate, via Request.System).
const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// SystemBlock is one system-prompt block (spec-1.20 R2 H-2: the real
// Anthropic system param is []TextBlockParam with per-block cache_control).
// spec-1.20 typically uses a single block; spec-2.10/2.11 three-layer
// prompt uses multiple blocks.
type SystemBlock struct {
	Text string
	// CacheBreak true → adapter sets cache_control: ephemeral on this
	// block (minimal prompt cache, spec-1.20 D-2). Full cache strategy
	// (memory incremental / hit-rate) is spec-3.10.
	CacheBreak bool
}

// BlockType tags a ContentBlock. spec-1.20 R2 H-6: append-only;
// spec-1.21 adds BlockToolResult. Switches on BlockType MUST be
// exhaustive with a default panic.
type BlockType int

// BlockType values (append-only; spec-1.21 adds BlockToolResult).
const (
	BlockText BlockType = iota
	BlockToolUse
	// BlockToolResult — spec-1.21 (multi-turn tool execution). Reserved.
)

// ContentBlock is a tagged union of message content (model-agnostic).
type ContentBlock struct {
	Type    BlockType
	Text    string   // Type == BlockText
	ToolUse *ToolUse // Type == BlockToolUse (decoded from assistant; D-4)
}

// Message is one conversation turn.
type Message struct {
	Role    Role
	Content []ContentBlock
}

// ToolSchema declares a tool the model may call (spec-1.20 D-4 schema
// only — no execution). InputSchema is a JSON Schema object map; the
// adapter builds the SDK ToolInputSchemaParam (Properties/Required) from it.
type ToolSchema struct {
	Name        string
	Description string
	InputSchema map[string]any
}

// ToolUse is a decoded tool-call request from the assistant. Input is
// map[string]any (schema-agnostic; spec-1.20 R2 MED-4: spec-1.21 consumes
// it, does not reshape). The adapter accumulates input_json_delta partial
// JSON per content-block index, then decodes with object/depth/size guard
// (D-4). spec-1.20 only DECODES; execution is spec-1.21.
type ToolUse struct {
	ID    string
	Name  string
	Input map[string]any
}

// ThinkingMode is the request-side extended-thinking switch (spec-1.20
// R2 H-3 / R2.1 MED). spec-1.20 supports Disabled/Enabled only; adaptive
// (dynamic budget) is deferred to spec-3.11 — config "adaptive" → the
// factory returns LLM.NOT_IMPLEMENTED.
type ThinkingMode int

// ThinkingMode values (spec-1.20 supports Disabled/Enabled; adaptive → spec-3.11).
const (
	ThinkingDisabled ThinkingMode = iota
	ThinkingEnabled
)

// Request is a model-agnostic completion request.
type Request struct {
	System    []SystemBlock // R2 H-2 (block-level cache_control); not a bare string
	Messages  []Message     // Role ∈ {User, Assistant}; "system" → LLM.REQUEST_INVALID
	Tools     []ToolSchema  // decode-only in spec-1.20
	MaxTokens int           // required > 0 (Anthropic; MaxTokens=0 prewarm not supported, ❌-13)
	// Temperature: nil → provider default; 0.0 is a VALID (deterministic)
	// value (R2 H-4 — real SDK uses param.Opt[float64], range [0,1]).
	Temperature    *float64
	ThinkingMode   ThinkingMode // Disabled/Enabled (R2.1)
	ThinkingBudget int          // tokens; Enabled requires ≥1024 and < MaxTokens (counts toward max_tokens)
}

// FinishReason mirrors the Anthropic stop_reason enum plus opendbx
// terminal states (spec-1.20 R2 H-6 — full enum, not a 3-value subset).
// domain-owned (does not depend on render-layer streaming.FinishReason);
// the app-layer bridge maps the renderable subset via mapToRender
// (T1 exhaustive test).
type FinishReason int

// FinishReason values (mirrors Anthropic stop_reason + opendbx terminal states).
const (
	FinishUnset        FinishReason = iota // still streaming
	FinishStop                             // end_turn
	FinishLength                           // max_tokens (痛点 1.1)
	FinishToolUse                          // tool_use
	FinishStopSequence                     // stop_sequence
	FinishPause                            // pause_turn (long task / server tool)
	FinishRefusal                          // refusal (NOT content_filter; R2 H-6)
	FinishCancelled                        // ctx cancel
	FinishError                            // error termination (Err carries partial)
)

// String returns a stable lowercase name for debug / status display.
func (f FinishReason) String() string {
	switch f {
	case FinishUnset:
		return "unset"
	case FinishStop:
		return "stop"
	case FinishLength:
		return "length"
	case FinishToolUse:
		return "tool_use"
	case FinishStopSequence:
		return "stop_sequence"
	case FinishPause:
		return "pause_turn"
	case FinishRefusal:
		return "refusal"
	case FinishCancelled:
		return "cancelled"
	case FinishError:
		return "error"
	}
	return "unknown"
}

// Terminal reports whether f is a stream-ending reason (anything but Unset).
func (f FinishReason) Terminal() bool { return f != FinishUnset }

// Chunk is one streamed delta. spec-1.20 R2 CRIT-2: Token (renderable
// text) flows to the spec-1.6 TokenStream; the control fields (Thinking /
// ToolUses / FinishReason) flow through the app-layer ctrl channel — they
// are NEVER packed into the FROZEN streaming.Chunk ({Token,FinishReason,Err}).
type Chunk struct {
	Token        string
	Thinking     bool // chunk is extended-thinking content (StripThink display control)
	FinishReason FinishReason
	ToolUses     []ToolUse // populated when FinishReason == FinishToolUse
	Err          error
}

// Stream is the iterator returned by Provider.Stream (spec-1.20 R2 — maps
// to the SDK's native *ssestream.Stream). Consume:
//
//	s, err := p.Stream(ctx, req)
//	if err != nil { ... }      // immediate error (auth / invalid / connect cancel)
//	defer s.Close()
//	for s.Next() { c := s.Chunk(); ... }
//	if err := s.Err(); err != nil { ... }  // terminal error
//
// Next returns false when the stream ends. MUST honor ctx: cancellation
// surfaces as Next returning false with Err()==context.Canceled (the
// adapter maps it; the app-layer classifies via finishFromErr).
type Stream interface {
	Next() bool   // advance; false = stream ended
	Chunk() Chunk // current chunk (valid after Next returns true)
	Err() error   // terminal error (nil = clean end)
	Close() error // release (defer; idempotent)
}

// Provider is the model-agnostic LLM gateway (规则 16). Implementations:
// anthropic (spec-1.20 D-2), fake (D-3), openai-compat (spec-3.11).
//
// Stream starts a streaming completion and returns an iterator. A nil
// error means the stream started; per-delta errors surface via
// Stream.Err(). req validation failure → LLM.REQUEST_INVALID. An
// unavailable / unauthenticated provider → LLM.UNAVAILABLE / AUTH_FAILED
// (NEVER a rule-based fallback — 原则 3 + AD-004 + R2 H-7).
type Provider interface {
	Name() string
	Stream(ctx context.Context, req Request) (Stream, error)
}

// ValidateRequest enforces the model-agnostic request invariants
// (spec-1.20 R2.1 MED): non-empty Messages, MaxTokens > 0, no "system"
// role inside Messages (system goes via Request.System), and a valid
// thinking budget when ThinkingEnabled. Adapters call this before mapping
// to SDK params so the error is a registered LLM.* code, not a raw SDK 400.
func ValidateRequest(req Request) error {
	if len(req.Messages) == 0 {
		return RequestInvalidf("empty Messages")
	}
	if req.MaxTokens <= 0 {
		return RequestInvalidf("MaxTokens must be > 0 (prewarm not supported)")
	}
	for _, m := range req.Messages {
		if m.Role != RoleUser && m.Role != RoleAssistant {
			return RequestInvalidf("Messages role must be user/assistant; system prompt goes via Request.System")
		}
	}
	if req.ThinkingMode == ThinkingEnabled {
		if req.ThinkingBudget < minThinkingBudget {
			return RequestInvalidf("ThinkingBudget must be ≥ 1024 when thinking enabled")
		}
		if req.ThinkingBudget >= req.MaxTokens {
			return RequestInvalidf("ThinkingBudget must be < MaxTokens (counts toward max_tokens)")
		}
	}
	return nil
}

// minThinkingBudget is the Anthropic minimum extended-thinking budget
// (spec-1.20 R2 H-3; SDK doc: enabled thinking requires ≥1024 tokens).
const minThinkingBudget = 1024
