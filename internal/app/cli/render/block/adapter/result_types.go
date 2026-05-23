// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File result_types.go — ToolResult-side adapter interfaces (spec-1.9b D-2).
// Append-only extension to spec-1.9 adapter package; HeaderRenderer /
// ProgressRenderer / QueuedRenderer (ToolUse-side; spec-1.9 FROZEN) are
// untouched. All 3 ToolResult-side interfaces are optional via Go type
// assertion (interface segregation per R2.1.3 HIGH-1 pattern).

package adapter

// ResultRenderer is the optional ToolResult-side render for SUCCESS path.
// Go port of CC Tool.ts:566-580 renderToolResultMessage (B-13).
//
// Adapter implementations:
//   - Bash: delegate to BashToolResultMessage-equivalent formatting
//     (spec-1.9b D-3 / B-14)
//   - Read: "Read N lines" / "image" / "PDF" etc. (D-3 / B-15
//     FileReadTool/UI.tsx:77-142)
//   - Generic: minimal "(success)" fallback (D-3 / Q3 ★A)
type ResultRenderer interface {
	RenderResult(content any, ctx Context) (string, error)
}

// RejectedRenderer is the optional ToolResult-side render for REJECTED.
// Go port of CC Tool.ts:641-653 renderToolUseRejectedMessage (B-13).
//
// Only Bash implements RejectedRenderer in spec-1.9b (per Q3 ★A);
// Read intentionally omits it; ToolResult.Render falls back to CC
// InterruptedByUser text when this interface is absent.
type RejectedRenderer interface {
	RenderRejected(content any, ctx Context) (string, error)
}

// ErrorResultRenderer is the optional ToolResult-side render for ERROR.
// Go port of CC Tool.ts:659-667 renderToolUseErrorMessage (B-13).
// Named "ErrorResultRenderer" not "ErrorRenderer" to avoid future
// collision with a hypothetical block-level ErrorRenderer.
//
// Adapter implementations:
//   - Bash: FallbackToolUseErrorMessage-equivalent generic content
//     display (D-3 / B-14 BashTool/UI.tsx:174-184; R2 MED-1 not custom
//     tail-of-stderr)
//   - Read: "File not found" / "Error reading file" (D-3 / B-15
//     FileReadTool/UI.tsx:144-164; R2 HIGH-2 — Read DOES implement
//     this, not only ResultRenderer)
//   - Generic: omitted; ToolResult.Render falls back to formatted Content
type ErrorResultRenderer interface {
	RenderErrorResult(content any, ctx Context) (string, error)
}
