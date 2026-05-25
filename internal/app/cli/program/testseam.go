// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package program

import "github.com/sqlrush/opendbx/internal/app/cli/render/buffer"

// RunRenderForTest is the spec-1.15 D-7/D-8 test seam used by the
// integration visualgolden harness. It directly invokes Program.renderFn
// against the supplied grid, optionally setting the quit-armed state so
// fixtures like ProgramQuitArmed can be captured deterministically.
//
// Test code only. Production callers MUST use Program.Run (which wires
// the scheduler main loop). Marked test-only via godoc — moving to
// _test.go would prevent reuse from sibling test packages, so the helper
// lives in production source but with an explicit name suffix.
func RunRenderForTest(p *Program, grid *buffer.Grid, quitArmed bool) {
	p.quitArmed = quitArmed
	p.renderFn(grid)
}
