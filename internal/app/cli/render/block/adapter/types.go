// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package adapter hosts per-tool render adapters consumed by
// block.ToolUse.Render. Mirrors CC Tool.ts:605-667 ToolUse subset
// (renderToolUseMessage / renderToolUseProgressMessage /
// renderToolUseQueuedMessage); ToolResult-side methods are owned by
// spec-1.9b (renderToolResultMessage / renderToolUseRejectedMessage /
// renderToolUseErrorMessage).
//
// Spec: spec-1.9-toolcall-block.md D-2 (R2 HIGH-4 + R2.1 HIGH-2 +
// R2.1.3 HIGH-1 — interface segregation: required + optional).
package adapter

// ThemeName is an opaque tag for the active CC theme, propagated from
// block.Context.Theme. Adapters consume it to pick palette overlays.
type ThemeName string

// Context is render-time information passed to per-tool adapters.
// Mirrors CC Tool.ts:605-608 options union (B-1 baseline).
type Context struct {
	Verbose      bool      // CC: options.verbose
	Theme        ThemeName // CC: options.theme
	Cols         int       // CC: options.terminalSize.columns
	IsTranscript bool      // CC: options.isTranscriptMode
	IsBriefOnly  bool      // CC: options.isBriefOnly (spec-1.10 compact 时 true)
}

// ProgressMessage mirrors CC ProgressMessage<P>; opendbx 字段子集.
// Used by ProgressRenderer for the Running branch.
type ProgressMessage struct {
	Text           string
	ElapsedSeconds int
	TotalLines     int
	TotalBytes     int
	TimeoutMs      int
}

// HeaderRenderer is the **required** per-tool render contract for
// **ToolUse only**. Go port of CC Tool.ts:605-608 renderToolUseMessage.
//
// Every tool MUST implement HeaderRenderer; the Registry stores values
// of this interface and ToolUse.Render dispatches via Registry.Lookup.
//
// Optional methods are exposed via ProgressRenderer and QueuedRenderer
// sub-interfaces; ToolUse.Render uses type assertion to test
// implementation (Go's idiomatic "optional method" pattern per
// spec-1.9 R2.1.3 HIGH-1).
//
// **Not in this interface** (owned by spec-1.9b ToolResult adapter):
//   - renderToolResultMessage       (success result render)
//   - renderToolUseRejectedMessage  (rejection UI)
//   - renderToolUseErrorMessage     (error result render)
type HeaderRenderer interface {
	// RenderHeader 渲染 tool-use header (≤ 1 row typical).
	// CC: Tool.ts:605-608 renderToolUseMessage; required.
	RenderHeader(input map[string]any, ctx Context) (string, error)
}

// ProgressRenderer is an optional interface. ToolUse.Render calls it
// for every StateRunning ToolUse when implemented, even when
// ProgressMessages is nil/empty. Bash uses the empty case to render
// the CC second row "Running…" (BashTool/UI.tsx:148).
//
// CC: Tool.ts:625-634 renderToolUseProgressMessage; optional.
type ProgressRenderer interface {
	RenderProgress(progress []ProgressMessage, ctx Context) (string, error)
}

// QueuedRenderer is an optional interface. ToolUse.Render calls it for
// StateQueued when implemented; otherwise falls back to "Waiting…"
// generic text (CC BashTool/UI.tsx:154-158).
//
// CC: Tool.ts:635 renderToolUseQueuedMessage; optional.
type QueuedRenderer interface {
	RenderQueued() (string, error)
}
