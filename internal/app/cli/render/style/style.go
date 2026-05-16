// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package style

import (
	"fmt"
	"strings"
)

// Color represents a foreground or background color slot. ColorDefault
// (zero value) means "use terminal default; emit no SGR color code".
// 0..15 → ANSI 16-color palette; 16..255 → 256-color extended palette;
// >= 0x1000000 → 24-bit truecolor encoded as 0x1RRGGBB (the high bit
// distinguishes from palette indices).
type Color uint32

const (
	// ColorDefault means "leave terminal default color".
	ColorDefault Color = 0
	// truecolorBit marks a 24-bit color; the lower 24 bits are RGB.
	truecolorBit Color = 0x1000000
)

// RGB packs an (r, g, b) triple into a truecolor Color.
func RGB(r, g, b uint8) Color {
	return truecolorBit | Color(r)<<16 | Color(g)<<8 | Color(b)
}

// Palette returns a 16-color palette Color (0..15).
func Palette(idx uint8) Color {
	return Color(idx) + 1 // +1 so ColorDefault (=0) stays distinct from palette index 0
}

// Style is the immutable attribute set for a single render cell.
type Style struct {
	FG, BG    Color
	Bold      bool
	Italic    bool
	Underline bool
	Reverse   bool
}

// ANSI returns the SGR escape sequence that would set this style on a
// terminal positioned at the cursor. Empty string when the style is
// the zero value (no attributes to set).
//
// MUST only be called by render/terminal (driver flush path). See
// package godoc invariant.
func (s Style) ANSI() string {
	var parts []string
	if s.Bold {
		parts = append(parts, "1")
	}
	if s.Italic {
		parts = append(parts, "3")
	}
	if s.Underline {
		parts = append(parts, "4")
	}
	if s.Reverse {
		parts = append(parts, "7")
	}
	if c := colorSGR(s.FG, true); c != "" {
		parts = append(parts, c)
	}
	if c := colorSGR(s.BG, false); c != "" {
		parts = append(parts, c)
	}
	if len(parts) == 0 {
		return ""
	}
	return "\x1b[" + strings.Join(parts, ";") + "m"
}

// Reset is the universal SGR reset escape, useful at end-of-frame to
// avoid bleed into shell prompt after Fini.
const Reset = "\x1b[0m"

// colorSGR emits the SGR fragment for a foreground (fg=true) or
// background slot. ColorDefault yields "".
func colorSGR(c Color, fg bool) string {
	if c == ColorDefault {
		return ""
	}
	if c >= truecolorBit {
		rgb := uint32(c) & 0xFFFFFF
		r := (rgb >> 16) & 0xFF
		g := (rgb >> 8) & 0xFF
		b := rgb & 0xFF
		prefix := "38;2"
		if !fg {
			prefix = "48;2"
		}
		return fmt.Sprintf("%s;%d;%d;%d", prefix, r, g, b)
	}
	// Palette index (subtract the +1 offset). Caller guarantees c < 256
	// when not truecolor (Palette constructor takes uint8).
	idx := uint8(c&0xFF) - 1 //nolint:gosec // c is bounded to 0..255 by Palette() ctor; truecolor branch handled above
	if idx < 8 {
		base := uint8(30)
		if !fg {
			base = 40
		}
		return fmt.Sprintf("%d", base+idx)
	}
	if idx < 16 {
		base := uint8(90)
		if !fg {
			base = 100
		}
		return fmt.Sprintf("%d", base+idx-8)
	}
	prefix := "38;5"
	if !fg {
		prefix = "48;5"
	}
	return fmt.Sprintf("%s;%d", prefix, idx)
}
