// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package input

import "github.com/sqlrush/opendbx/internal/app/cli/render/style"

// StyleFor returns the style.Style for the given mode. Used by
// program.paintInputRow to color the input row.
//
// spec-1.16 D-6 (R2 H-1 fix): input-local theme map; NOT a
// render/block.StyleTheme implementation (DAG isolation). Returns
// style.Style{} (terminal default) for ModeNatural.
//
// R4 MED-1 (go-reviewer post-impl): the prior exported StyleKind enum
// + constants were removed — no external consumer, and exporting them
// created interface-pollution with 0% test coverage on the symbols.
// StyleFor is the single public API for mode→style resolution; if a
// future spec needs a theme abstraction, re-export with a real consumer.
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
