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
// The companion ErrOutOfBounds sentinel below documents that contract
// in errcode form for callers that wish to log or instrument it.
//
//nolint:gochecknoglobals // spec-0.6 contract: errcode sentinels are package-level.
var ErrInvalidDimension = errcode.Register(
	"RENDER.INVALID_DIMENSION",
	"buffer grid received an invalid dimension",
	"check cols > 0 and rows > 0 from terminal resize event, and ensure cols × rows does not overflow int32 (~2.1×10^9 cells, far above any practical viewport)",
)

// ErrOutOfBounds is the errcode sentinel documenting Grid.Cell/Grid.SetCell
// out-of-bounds contract (spec-0.13 D-7 forward-declared; registration
// deferred to this package per spec-1.2 R3 errata). It is NOT returned by
// Cell/SetCell — those keep the spec-0.13 D-3 `void`/zero-value signatures
// for forward-compat — but callers (e.g. spec-1.3 optimizer, spec-1.4
// scheduler) that want to log or instrument OOB writes can reference this
// sentinel by Code() to stay aligned with the registry.
//
//nolint:gochecknoglobals // spec-0.6 contract: errcode sentinels are package-level.
var ErrOutOfBounds = errcode.Register(
	"RENDER.OUT_OF_BOUNDS",
	"buffer grid coordinates outside Size()",
	"call Buffer.Size() and clamp x ∈ [0, cols), y ∈ [0, rows) before SetCell/Cell; or call Buffer.Resize(cols, rows) if the viewport actually grew",
)
