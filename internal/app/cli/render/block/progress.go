// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package block

import "github.com/sqlrush/opendbx/internal/app/cli/render/buffer"

// Progress is the spec-0.13 stub for the progress block type. Render returns
// (nil, ErrUnsupportedNode) per the spec-0.13 D-3 contract (replaces R1
// panic path; R2 codex HIGH-5).
//
// TODO: implement (spec-1.7+ progress-block; spec-1.x slug pending)
type Progress struct{}

// Render satisfies the RenderNode interface but returns the unsupported
// sentinel for this stub.
func (Progress) Render(_ Context) (buffer.Buffer, error) {
	// errcode-lint:exempt -- spec-0.13 D-3: ErrUnsupportedNode is the registered sentinel; spec-1.7+ replaces with real Render.
	return nil, ErrUnsupportedNode
}
