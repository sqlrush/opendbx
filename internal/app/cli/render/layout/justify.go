// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package layout

// Justify is the main-axis distribution rule for a flex container's
// children (CSS `justify-content`). spec-1.1 D-2 ships 5 values
// covering the CC Ink Box.tsx surface minus `space-evenly` (❌-7
// deferred to a future errata when a real CC UI block needs it).
type Justify uint8

const (
	// JustifyStart packs children at the main-axis start. CC Ink default.
	JustifyStart Justify = iota
	// JustifyCenter centers children on the main axis: any free space
	// is split equally before and after the group.
	JustifyCenter
	// JustifyEnd packs children at the main-axis end.
	JustifyEnd
	// JustifySpaceBetween distributes children with equal gaps between
	// them; the first and last child touch the parent edges. With fewer
	// than 2 children, falls back to JustifyStart (no gap well-defined).
	JustifySpaceBetween
	// JustifySpaceAround distributes children with equal gaps on both
	// sides of every child; edge gaps are half the inter-child gap.
	JustifySpaceAround
)

// applyJustify returns (leadingSpace, interChildGap) for laying out n
// children whose combined main size is totalMain inside parentMain.
//
// JustifySpaceBetween with n < 2 falls back to JustifyStart because
// a single child has no gap to fill. JustifySpaceAround with n == 0
// returns (0, 0). Inter-child gaps are integer-truncated; the remainder
// stays at the trailing edge of the parent so the result is
// deterministic and reproducible for golden-file tests.
func applyJustify(justify Justify, parentMain, totalMain, n int) (leading, gap int) {
	free := parentMain - totalMain
	if free < 0 {
		free = 0
	}
	switch justify {
	case JustifyStart:
		return 0, 0
	case JustifyCenter:
		return free / 2, 0
	case JustifyEnd:
		return free, 0
	case JustifySpaceBetween:
		if n < 2 {
			return 0, 0
		}
		return 0, free / (n - 1)
	case JustifySpaceAround:
		if n == 0 {
			return 0, 0
		}
		// Each child gets `unit` half-gap on either side; total gaps =
		// 2n half-units = n full units of size `gap = 2 * unit`.
		unit := free / (2 * n)
		return unit, unit * 2
	default:
		return 0, 0
	}
}
