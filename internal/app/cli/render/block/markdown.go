// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package block

import "github.com/sqlrush/opendbx/internal/app/cli/render/buffer"

// Markdown is the spec-0.13 stub for the markdown block type. Render returns
// (nil, ErrUnsupportedNode) per the spec-0.13 D-3 contract (replaces R1
// panic path; R2 codex HIGH-5).
//
// TODO(spec-X.Y): replace stub Render with real implementation; see spec-1.7 D-6 R2 D6 deferred-stub contract.
type Markdown struct{}

// Render satisfies the RenderNode interface but returns the unsupported
// sentinel for this stub.
func (Markdown) Render(_ Context) (buffer.Buffer, error) {
	// errcode-lint:exempt -- spec-0.13 D-3: ErrUnsupportedNode is the registered sentinel; spec-1.7+ replaces with real Render.
	return nil, ErrUnsupportedNode
}
