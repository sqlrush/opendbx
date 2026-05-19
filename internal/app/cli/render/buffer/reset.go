// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package buffer

// Reset clears the Grid in O(1) by incrementing the generation
// counter (spec-1.2 D-3). Subsequent reads via Cell(x, y) return
// the zero Cell{} until a SetCell at that coordinate stamps the
// current generation into cellGen[idx].
//
// Overflow fallback (spec-1.2 R2 MED-1): the uint64 generation
// counter has a wraparound period of ~9.7 × 10^9 years at 60 fps —
// not reachable in practice, but the contract MUST be defined to
// avoid stale cellGen == 0 slots being mis-read as "live" if the
// counter ever rolls over. On wraparound the cells / cellGen slices
// are explicitly cleared and generation restarts at 1 (matching the
// NewGrid initial state where cellGen[i] == 0 is "no cell" and
// generation == 1 is the first live frame).
func (g *Grid) Reset() {
	g.generation++
	if g.generation == 0 {
		for i := range g.cells {
			g.cells[i] = Cell{}
		}
		for i := range g.cellGen {
			g.cellGen[i] = 0
		}
		g.generation = 1
	}
}
