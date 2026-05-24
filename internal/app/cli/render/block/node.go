// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package block defines the RenderNode interface for all user-visible
// rendered blocks (message / toolcall / toolresult / compact / markdown /
// code / diff / banner / progress) and the remaining unsupported stubs.
//
// Each stub Render() returns (nil, ErrUnsupportedNode) — spec-1.7+
// fills the real implementation per block type. Renaming this from R1's
// panic-stub to error-return path was R2 codex HIGH-5: errors flow
// through the call chain instead of crashing the process.
//
// DAG position: render/block is index 7 (depends on render/layout +
// render/buffer + render/width + render/style).
//
// Design: spec-0.13-render-engine-skeleton § 2.1 (D-1) + § 2.3 (D-3)
package block

import (
	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

// Context carries cross-cutting render state passed to every block.Render
// call. spec-0.13 D-3 shipped a placeholder; spec-1.7 D-1 extends with
// Theme / MeasureOnly / Wrap production fields per spec-1.6 forward and
// spec-1.5 R-2 forward.
//
// **MeasureOnly** (spec-1.5 R-2 forward + spec-1.6 R-9): when true, Render
// returns a Buffer whose Size() reports the real rows but cell writes are
// no-ops. spec-1.5 scrollback's measureHeight + rebuildHeights cheap path.
//
// **Theme** (spec-1.7 D-1): style palette for Normal / Dimmed / Warning /
// Code / CodeBg / LangLabel. Theme.Style(kind) returns a style.Style.
// nil Theme = use DefaultTheme (spec-1.7 R3 fixture lock-in).
//
// **Wrap** (spec-1.7 D-1): text wrap policy. Hard / Soft (default; CJK-
// aware word break) / None (single-line truncate). See WrapPolicy godoc.
//
// **Cols / Rows** (existing): viewport size in cells. Caller (spec-1.5
// scrollback / spec-1.14 TUI) converts layout.Box.Width/Height → Cols/Rows
// (spec-1.7 R2 D7 LOW-1: block does NOT directly import layout).
type Context struct {
	Cols, Rows  int
	Theme       StyleTheme // nil → DefaultTheme; spec-1.7 D-1
	MeasureOnly bool       // spec-1.5 R-2 forward + spec-1.6 R-9
	Wrap        WrapPolicy // default Soft; spec-1.7 D-1
	Verbose     bool       // spec-1.7 R3 forward / spec-1.9 D-6 + Q4 ★A —
	// transcript-mode verbose flag injected by spec-1.15 TUI; adapters use
	// it for verbose vs condensed rendering. Default false preserves
	// spec-1.7 Message.Render legacy behavior.
	IsTranscript bool // **spec-1.10 D-6 forward errata (spec-1.7 R4)** — CC
	// Ctrl+O `app:toggleTranscript` 全局 mode (per spec-1.9 R2 HIGH-3 / B-9).
	// Injected by spec-1.15 TUI per-tick. block.CompactSummary.Render gates
	// on this OR Verbose: when EITHER is true, expanded mode → 0 rows
	// (caller renders 原 ToolUse/ToolResult); default false 不破 spec-1.7
	// Message / spec-1.9 ToolUse / spec-1.9b ToolResult FROZEN 行为.

	// ColorDepth is the rendering target's color capability injected by
	// the TUI caller (spec-1.15) via tcell `Screen.Colors()` probe.
	// **spec-1.12 D-5 / CRIT-2 ★A forward errata (spec-1.7 R4)**.
	// Values:
	//   0          — unset (zero value); treated as 16-color conservative
	//                fallback until spec-1.15 injects real capability
	//                (R3 HIGH-2 user 拍板 safer default).
	//   16         — legacy 16-color terminal.
	//   256        — xterm-256color.
	//   16777216   — 24-bit truecolor.
	// Block layer renderers (specifically spec-1.12 D-6 mapChromaColor)
	// use this to choose RGB vs nearest-palette downgrade. Other block
	// types ignore it (forward-compat — only render paths needing
	// ColorDepth read this field).
	ColorDepth int
}

// RenderNode is the contract every block type implements. Render produces
// a Buffer of (rune, style) cells suitable for paste into the scrollback
// or streaming output. Returns (nil, ErrUnsupportedNode) for the spec-0.13
// stub types; spec-1.7+ replaces stubs with real implementations.
//
// Return-type design note (spec-0.13 T-13 go-reviewer R1 MED-1): Render
// returns the Buffer interface (not a concrete struct) intentionally —
// spec-1.3 cell-grid-buffer plans to swap between allocating vs
// sync.Pool-backed impls without breaking block-type implementors. This
// is a deliberate deviation from the "accept interfaces, return structs"
// Go idiom and is logged here so future refactors don't undo it.
type RenderNode interface {
	Render(ctx Context) (buffer.Buffer, error)
}

// ErrUnsupportedNode is returned by the remaining unsupported block stubs.
// Callers should check `errors.Is(err, ErrUnsupportedNode)` and surface
// the actionable hint to the user.
//
//nolint:gochecknoglobals // spec-0.6 contract: errcode sentinels are package-level.
var ErrUnsupportedNode = errcode.Register(
	"RENDER.UNSUPPORTED_NODE",
	"block.Render called on unimplemented block type",
	"this block type is not yet implemented; see spec-1.7+ block-type specs (message / toolcall / toolresult / compact already implemented; markdown / code / diff / banner / progress pending)",
)
