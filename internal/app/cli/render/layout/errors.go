// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package layout

import "github.com/sqlrush/opendbx/internal/platform/errcode"

// ErrInvalidDimension is returned by Layouter.Layout when input
// constraints are violated:
//
//   - viewport.Width or viewport.Height ≤ 0
//   - any FlexNode.Grow or FlexNode.Shrink < 0
//   - any FlexNode.Basis < 0 when BasisMode == BasisFixed
//   - any leaf's Intrinsic() callback returns a negative value
//   - any container has more than MaxChildren (1000) children
//   - cumulative main-axis sum overflows int32
//
// All checks are CLAUDE.md rule 7 error 三件套: Code/Message/Hint.
// The Hint enumerates the most likely cause so callers can correct the
// input tree before retrying.
//
//nolint:gochecknoglobals // spec-0.6 contract: errcode sentinels are package-level.
var ErrInvalidDimension = errcode.Register(
	"RENDER.INVALID_DIMENSION",
	"flex layout received an invalid dimension or tree shape",
	"check viewport > 0, grow/shrink ≥ 0, basis ≥ 0, intrinsic ≥ 0, children ≤ 1000 per container, and no main-axis sum overflow",
)

// ErrLayoutCycle is returned by Layouter.Layout when an Intrinsic()
// callback re-enters the layout measurement of the same Node
// (A.Intrinsic() → B.Intrinsic() → A.Intrinsic()). spec-1.1 R2-8
// explicitly scopes cycle detection to measurement callback re-entry
// only; child graph cycles (a Children list that loops back on
// itself) are caller responsibility — the Node interface exposes no
// Children() API for the layout side to enumerate, so the cycle would
// manifest as unbounded recursion in caller-controlled code.
//
//nolint:gochecknoglobals // spec-0.6 contract: errcode sentinels are package-level.
var ErrLayoutCycle = errcode.Register(
	"RENDER.LAYOUT_CYCLE",
	"flex layout detected an intrinsic-callback re-entry cycle",
	"ensure each leaf's Intrinsic callback does not recursively invoke layout measurement on itself or on a node that transitively measures it back",
)
