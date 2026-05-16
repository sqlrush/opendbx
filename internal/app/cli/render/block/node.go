// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package block defines the RenderNode interface for all user-visible
// rendered blocks (message / toolcall / compact / markdown / code / diff
// / banner / progress) and provides 8 type stubs.
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
// call. spec-0.13 D-3 ships a placeholder; spec-1.7+ extends with
// viewport / theme / format flags.
type Context struct {
	Cols, Rows int
	// Future: Theme, OutputFormat, etc.
}

// RenderNode is the contract every block type implements. Render produces
// a Buffer of (rune, style) cells suitable for paste into the scrollback
// or streaming output. Returns (nil, ErrUnsupportedNode) for the spec-0.13
// stub types; spec-1.7+ replaces stubs with real implementations.
type RenderNode interface {
	Render(ctx Context) (buffer.Buffer, error)
}

// ErrUnsupportedNode is returned by the 8 type stubs in spec-0.13.
// Callers should check `errors.Is(err, ErrUnsupportedNode)` and surface
// the actionable hint to the user.
//
//nolint:gochecknoglobals // spec-0.6 contract: errcode sentinels are package-level.
var ErrUnsupportedNode = errcode.Register(
	"RENDER.UNSUPPORTED_NODE",
	"block.Render called on unimplemented block type",
	"this block type is not yet implemented; see spec-1.7+ block-type specs (message / toolcall / markdown / code / diff / banner / progress / compact)",
)
