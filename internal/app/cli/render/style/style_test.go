// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package style

import "testing"

func TestStyle_ZeroValueEmpty(t *testing.T) {
	t.Parallel()
	if (Style{}).ANSI() != "" {
		t.Errorf("zero Style.ANSI() = %q, want empty", (Style{}).ANSI())
	}
}

func TestStyle_Bold(t *testing.T) {
	t.Parallel()
	if got := (Style{Bold: true}).ANSI(); got != "\x1b[1m" {
		t.Errorf("Bold ANSI = %q, want \\x1b[1m", got)
	}
}

func TestStyle_AllFlags(t *testing.T) {
	t.Parallel()
	got := Style{Bold: true, Italic: true, Underline: true, Reverse: true}.ANSI()
	want := "\x1b[1;3;4;7m"
	if got != want {
		t.Errorf("all-flags = %q, want %q", got, want)
	}
}

func TestStyle_PaletteFGBG(t *testing.T) {
	t.Parallel()
	// Palette(0) → +1 offset → palette index 0 → SGR 30 (FG) / 40 (BG).
	got := Style{FG: Palette(0), BG: Palette(7)}.ANSI()
	want := "\x1b[30;47m"
	if got != want {
		t.Errorf("palette FG=0/BG=7 = %q, want %q", got, want)
	}
}

func TestStyle_BrightPalette(t *testing.T) {
	t.Parallel()
	// Palette(8) → bright black FG (90).
	got := Style{FG: Palette(8)}.ANSI()
	if got != "\x1b[90m" {
		t.Errorf("bright FG = %q, want \\x1b[90m", got)
	}
}

func TestStyle_Color256(t *testing.T) {
	t.Parallel()
	got := Style{FG: Palette(196)}.ANSI() // 256-color red
	want := "\x1b[38;5;196m"
	if got != want {
		t.Errorf("256-color FG = %q, want %q", got, want)
	}
}

func TestStyle_TrueColor(t *testing.T) {
	t.Parallel()
	got := Style{FG: RGB(255, 128, 0)}.ANSI()
	want := "\x1b[38;2;255;128;0m"
	if got != want {
		t.Errorf("RGB FG = %q, want %q", got, want)
	}
}

func TestStyle_TrueColorBG(t *testing.T) {
	t.Parallel()
	got := Style{BG: RGB(0, 0, 255)}.ANSI()
	want := "\x1b[48;2;0;0;255m"
	if got != want {
		t.Errorf("RGB BG = %q, want %q", got, want)
	}
}

func TestReset_Constant(t *testing.T) {
	t.Parallel()
	if Reset != "\x1b[0m" {
		t.Errorf("Reset = %q, want \\x1b[0m", Reset)
	}
}
