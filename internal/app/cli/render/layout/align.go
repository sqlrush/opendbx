// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package layout

// Align is the cross-axis alignment rule for a flex container's children
// (CSS `align-items`). spec-1.1 D-2 ships 4 values; CC Ink's
// `alignSelf` per-child override (styles.ts L197) is deferred to
// spec-1.7+ block-individual overrides (❌-8).
type Align uint8

const (
	// AlignStretch stretches each child to fill the parent's cross axis.
	// CC Ink default (matches W3C flexbox default).
	AlignStretch Align = iota
	// AlignStart aligns each child to the cross-axis start; child keeps
	// its intrinsic cross size (clamped to parent cross).
	AlignStart
	// AlignCenter centers each child on the cross axis.
	AlignCenter
	// AlignEnd aligns each child to the cross-axis end.
	AlignEnd
)

// applyAlign returns (crossSize, crossOffset) for a single child.
//
// crossSize is the child's cross dimension after alignment processing.
// crossOffset is added to the parent's cross origin to position the
// child along the cross axis.
//
// AlignStretch always returns (parentCross, 0). The non-stretch modes
// clamp the child's intrinsic cross to parentCross via clampCross;
// a zero intrinsic falls back to parentCross so a leaf without a
// Measure callback still appears visible.
func applyAlign(align Align, parentCross, childIntrinsicCross int) (crossSize, crossOffset int) {
	switch align {
	case AlignStretch:
		return parentCross, 0
	case AlignStart:
		return clampCross(childIntrinsicCross, parentCross), 0
	case AlignCenter:
		size := clampCross(childIntrinsicCross, parentCross)
		return size, (parentCross - size) / 2
	case AlignEnd:
		size := clampCross(childIntrinsicCross, parentCross)
		return size, parentCross - size
	default:
		return parentCross, 0
	}
}

// clampCross returns childCross clamped to parentCross (and ≥ 0).
// A zero or negative childCross falls back to parentCross (visible
// default for leaves without an Intrinsic); a childCross larger than
// parentCross is clipped to parentCross (no overflow on cross axis).
func clampCross(childCross, parentCross int) int {
	if childCross <= 0 {
		return parentCross
	}
	if childCross > parentCross {
		return parentCross
	}
	return childCross
}
