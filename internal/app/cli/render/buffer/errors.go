// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package buffer

import "github.com/sqlrush/opendbx/internal/platform/errcode"

// ErrInvalidDimension is returned by NewGrid and BufferPool.Acquire
// when input dimensions are invalid: cols ≤ 0, rows ≤ 0, or
// cols × rows overflows int32 (which would make allocating the
// backing slices unsafe on 32-bit and an attack vector on 64-bit).
//
// CLAUDE.md rule 7 error 三件套 (Code / Message / Hint).
//
// Out-of-bounds reads (Cell) and writes (SetCell) on a valid Grid
// do NOT return this error — they are silently handled per the
// spec-0.13 D-3 Buffer interface contract (Cell returns zero Cell{},
// SetCell is a no-op) to avoid panicking the render goroutine.
//
//nolint:gochecknoglobals // spec-0.6 contract: errcode sentinels are package-level.
var ErrInvalidDimension = errcode.Register(
	"RENDER.INVALID_DIMENSION",
	"buffer grid received an invalid dimension",
	"check cols > 0 and rows > 0 from terminal resize event, and ensure cols × rows does not overflow int32 (~2.1×10^9 cells, far above any practical viewport)",
)
