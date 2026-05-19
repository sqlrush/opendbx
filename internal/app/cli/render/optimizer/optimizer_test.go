// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package optimizer

import (
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
)

// TestPatchKinds_Distinct — pairwise distinct check including the
// spec-1.3 D-3 PatchResize addition.
func TestPatchKinds_Distinct(t *testing.T) {
	t.Parallel()
	kinds := []PatchKind{PatchSetCell, PatchMoveCursor, PatchStyleChange, PatchResize}
	for i := 0; i < len(kinds); i++ {
		for j := i + 1; j < len(kinds); j++ {
			if kinds[i] == kinds[j] {
				t.Errorf("PatchKind collision at index (%d,%d): both = %d", i, j, kinds[i])
			}
		}
	}
}

// TestPatchKinds_Values — spec-1.3 R2 G2 hard guard: lock down concrete
// numeric values so the data contract cannot drift if spec-1.3a inserts
// a new PatchKind between existing ones. PatchSetCell=0 / PatchMoveCursor=1
// / PatchStyleChange=2 are spec-0.13 D-1 FROZEN; PatchResize=3 is spec-1.3
// D-3 (appended in same const block via bare iota carryover).
func TestPatchKinds_Values(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		got  PatchKind
		want PatchKind
	}{
		{"PatchSetCell", PatchSetCell, 0},
		{"PatchMoveCursor", PatchMoveCursor, 1},
		{"PatchStyleChange", PatchStyleChange, 2},
		{"PatchResize", PatchResize, 3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if c.got != c.want {
				t.Errorf("%s = %d, want %d (data contract violation; spec-1.3 R2 G2)",
					c.name, c.got, c.want)
			}
		})
	}
}

// TestOptimizer_InterfaceContract — *DiffEngine satisfies the Optimizer
// interface. Uses a real 1×1 Grid for next (next MUST be non-nil per
// spec-1.3 R-10 precondition; spec-1.2 R3 codex MED-1 fix replaces the
// old fakeOptimizer Diff(nil, nil) call). prev remains nil to exercise
// the fullRedraw clean-surface path; an empty 1×1 Grid produces zero
// patches under G1.
func TestOptimizer_InterfaceContract(t *testing.T) {
	t.Parallel()
	g, err := buffer.NewGrid(1, 1)
	if err != nil {
		t.Fatalf("NewGrid: %v", err)
	}
	var o Optimizer = NewDiffEngine()
	patches := o.Diff(nil, g)
	if len(patches) != 0 {
		t.Errorf("nil-prev empty-next expected 0 patches; got %+v", patches)
	}
}
