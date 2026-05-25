// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package input

import "github.com/sqlrush/opendbx/internal/app/cli/render/style"

// StyleKind is the input-package-local style enum for mode-aware input
// row rendering. spec-1.16 D-6 (R2 H-1 fix): defined HERE rather than
// extending render/block.StyleKind, because block sits at DAG index 7
// and input sits at DAG index 9.5 — block has no knowledge of mode
// rendering, and extending block.StyleKind from input would violate
// the leaf→root sequence (block(7) imports nothing higher than itself).
type StyleKind int

const (
	// StyleInputNatural is the default style for natural-language input.
	// Equivalent to terminal default (no FG/BG override).
	StyleInputNatural StyleKind = iota

	// StyleInputSlash highlights slash-mode buffer. Cyan FG (CC slash
	// command 风格 parity).
	StyleInputSlash

	// StyleInputSQL highlights SQL-mode buffer. Dim green FG (psql parity).
	StyleInputSQL
)

// String returns the kind name for debug.
func (k StyleKind) String() string {
	switch k {
	case StyleInputSlash:
		return "input-slash"
	case StyleInputSQL:
		return "input-sql"
	default:
		return "input-natural"
	}
}

// StyleFor returns the style.Style for the given mode. Used by
// program.paintInputRow to color the input row.
//
// R2 D-6 (spec-1.16): input-local theme map; not a render/block.StyleTheme
// implementation (DAG isolation).
func StyleFor(mode Mode) style.Style {
	switch mode {
	case ModeSlash:
		return style.Style{FG: style.RGB(0x00, 0xC0, 0xC0)} // cyan
	case ModeSQL:
		return style.Style{FG: style.RGB(0x50, 0xC8, 0x50)} // dim green (psql parity)
	default:
		return style.Style{} // terminal default
	}
}
