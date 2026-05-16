// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package optimizer

import (
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
)

func TestPatchKinds_Distinct(t *testing.T) {
	t.Parallel()
	if PatchSetCell == PatchMoveCursor || PatchSetCell == PatchStyleChange || PatchMoveCursor == PatchStyleChange {
		t.Errorf("patch kind constants collide")
	}
}

type fakeOptimizer struct{}

func (fakeOptimizer) Diff(_, _ buffer.Buffer) []Patch {
	return []Patch{{Kind: PatchSetCell, X: 0, Y: 0, Cell: buffer.Cell{Ch: 'X'}}}
}

func TestOptimizer_InterfaceContract(t *testing.T) {
	t.Parallel()
	var o Optimizer = fakeOptimizer{}
	patches := o.Diff(nil, nil)
	if len(patches) != 1 || patches[0].Cell.Ch != 'X' {
		t.Errorf("Diff: %+v", patches)
	}
}
