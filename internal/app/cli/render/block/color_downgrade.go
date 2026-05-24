// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File color_downgrade.go — spec-1.12 D-6 chroma color → opendbx
// style.Color downgrade. Per R2 CRIT-2 ★A + R3 HIGH-2 ★A user 拍板:
//
//   - ctx.ColorDepth = 16777216 → truecolor (RGB direct).
//   - ctx.ColorDepth = 256       → nearest of xterm-256 palette.
//   - ctx.ColorDepth = 16        → nearest of 16-color palette.
//   - ctx.ColorDepth = 0         → unset; conservative fallback to 16
//     (NOT truecolor, per R3 HIGH-2 — safer to assume low cap until
//     spec-1.15 TUI injects real capability via tcell `Screen.Colors()`).
//
// CC color-diff (B-36 verified) selects truecolor only on
// `COLORTERM=truecolor|24bit`, else color256; opendbx errs more
// conservatively for unknown.
//
// chroma `Colour.Distance()` (Euclidean RGB distance) picks nearest
// palette entry. LUTs are built once via sync.Once.

package block

import (
	"sync"

	"github.com/sqlrush/opendbx/internal/app/cli/render/style"

	"github.com/alecthomas/chroma/v2"
)

// mapChromaColor downgrades a chroma 24-bit RGB color to the closest
// opendbx style.Color given the target color depth.
//
// If c is unset (chroma.NewColour zero), returns 0 (default color —
// caller may treat as "inherit"). Truecolor path is loss-less.
func mapChromaColor(c chroma.Colour, depth int) style.Color {
	if !c.IsSet() {
		return 0
	}
	switch depth {
	case 16777216:
		return style.RGB(c.Red(), c.Green(), c.Blue())
	case 256:
		return nearest256(c)
	case 16, 0:
		// 0 = unset → conservative 16-color fallback (R3 HIGH-2 ★A).
		return nearest16(c)
	default:
		// Unknown depth value — treat as conservative 16.
		return nearest16(c)
	}
}

// nearest256 returns the nearest xterm-256 palette index (16..255).
// Excludes 0..15 (system colors handled by 16-color path) plus 256-
// color extended grayscale + 6x6x6 color cube.
func nearest256(c chroma.Colour) style.Color {
	palette256LUT.once.Do(palette256LUT.build)
	bestIdx := uint8(16)
	bestDist := float64(1<<31 - 1)
	for i, pc := range palette256LUT.colors {
		d := c.Distance(pc)
		if float64(d) < bestDist {
			bestDist = float64(d)
			// i ∈ [0, 239]; i+16 ∈ [16, 255] — fits uint8.
			bestIdx = uint8(i + 16) //nolint:gosec // spec-1.12 D-6: bounded by 240-entry LUT (i+16 ∈ [16,255] fits uint8)
		}
	}
	return style.Palette(bestIdx)
}

// nearest16 returns the nearest 16-color palette index (0..15).
func nearest16(c chroma.Colour) style.Color {
	palette16LUT.once.Do(palette16LUT.build)
	bestIdx := uint8(0)
	bestDist := float64(1<<31 - 1)
	for i, pc := range palette16LUT.colors {
		d := c.Distance(pc)
		if float64(d) < bestDist {
			bestDist = float64(d)
			// i ∈ [0, 15] — fits uint8.
			bestIdx = uint8(i) //nolint:gosec // spec-1.12 D-6: bounded by 16-entry LUT (i ∈ [0,15] fits uint8)
		}
	}
	return style.Palette(bestIdx)
}

// paletteLUT holds a chroma.Colour slice for nearest-search.
type paletteLUT struct {
	once   sync.Once
	colors []chroma.Colour
}

// palette16LUT is the 16-color xterm/ANSI base palette. RGB values
// per de-facto xterm/macOS Terminal mapping.
var palette16LUT = &paletteLUT{}

func (p *paletteLUT) build() {
	if p == palette16LUT {
		p.colors = []chroma.Colour{
			chroma.NewColour(0x00, 0x00, 0x00), // 0  black
			chroma.NewColour(0xcd, 0x00, 0x00), // 1  red
			chroma.NewColour(0x00, 0xcd, 0x00), // 2  green
			chroma.NewColour(0xcd, 0xcd, 0x00), // 3  yellow
			chroma.NewColour(0x00, 0x00, 0xee), // 4  blue
			chroma.NewColour(0xcd, 0x00, 0xcd), // 5  magenta
			chroma.NewColour(0x00, 0xcd, 0xcd), // 6  cyan
			chroma.NewColour(0xe5, 0xe5, 0xe5), // 7  white
			chroma.NewColour(0x7f, 0x7f, 0x7f), // 8  bright black
			chroma.NewColour(0xff, 0x00, 0x00), // 9  bright red
			chroma.NewColour(0x00, 0xff, 0x00), // 10 bright green
			chroma.NewColour(0xff, 0xff, 0x00), // 11 bright yellow
			chroma.NewColour(0x5c, 0x5c, 0xff), // 12 bright blue
			chroma.NewColour(0xff, 0x00, 0xff), // 13 bright magenta
			chroma.NewColour(0x00, 0xff, 0xff), // 14 bright cyan
			chroma.NewColour(0xff, 0xff, 0xff), // 15 bright white
		}
		return
	}
	// 256-palette path: build the 6x6x6 color cube (16..231) +
	// grayscale ramp (232..255).
	colors := make([]chroma.Colour, 0, 240)
	cubeAxis := []byte{0x00, 0x5f, 0x87, 0xaf, 0xd7, 0xff}
	for r := 0; r < 6; r++ {
		for g := 0; g < 6; g++ {
			for b := 0; b < 6; b++ {
				colors = append(colors, chroma.NewColour(cubeAxis[r], cubeAxis[g], cubeAxis[b]))
			}
		}
	}
	// grayscale 232..255 (24 steps).
	for i := 0; i < 24; i++ {
		gray := byte(0x08 + i*10)
		colors = append(colors, chroma.NewColour(gray, gray, gray))
	}
	p.colors = colors
}

// palette256LUT is the xterm 256-color extended palette (entries 16..255).
var palette256LUT = &paletteLUT{}
