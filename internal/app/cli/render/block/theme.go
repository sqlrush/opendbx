// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File theme.go — StyleTheme interface + StyleKind enum + WrapPolicy
// enum + DefaultTheme. spec-1.7 D-1.
//
// Theme abstraction lets future spec-1.x runtime switching (light/dark
// mode, user config) plug in without breaking block.Message.Render
// callers. MVP ships DefaultTheme matching CC visual fixture (spec-1.7
// R2 D5 T-2.5 fixture lock-in; placeholder until T-9 R3 fixture verify).

package block

import "github.com/sqlrush/opendbx/internal/app/cli/render/style"

// StyleKind names the semantic style slots used by block renderers.
// spec-1.7 D-1 + R2 D1 (CRIT-1 Continued StyleDimmed vs Truncated
// StyleWarning distinction).
type StyleKind int

const (
	// StyleNormal — default text style (no emphasis).
	StyleNormal StyleKind = iota
	// StyleDimmed — Continued continuation marker; "(no output)" Empty
	// placeholder (spec-1.7 R2 D1 + 痛点 1.5).
	StyleDimmed
	// StyleWarning — Truncated marker (FinishLength / FinishCancelled /
	// FinishError surfaces; spec-1.6 D-5 + spec-1.7 R2 D1).
	StyleWarning
	// StyleCode — code fence body foreground.
	StyleCode
	// StyleCodeBg — code fence cell background fill.
	StyleCodeBg
	// StyleLangLabel — fence lang label decoration (e.g. `─── go ───`).
	StyleLangLabel
)

// StyleTheme provides the palette resolution from semantic StyleKind to
// concrete style.Style. Implementations: DefaultTheme (MVP); future
// LightTheme / DarkTheme / UserCustomTheme (spec-1.x runtime switch).
type StyleTheme interface {
	Style(kind StyleKind) style.Style
}

// WrapPolicy controls text line-wrap behavior in block.Message.Render.
// spec-1.7 D-1 Q1 ★A: default Soft (CJK-aware word break).
type WrapPolicy int

const (
	// WrapSoft — prefer last ASCII space whose visual width fits; hard-
	// break a single word longer than cols; never split wide rune /
	// combining cluster; ignore ANSI bytes for width (R2 D2 删 ANSI
	// scope — current MVP treats ANSI as raw chars).
	WrapSoft WrapPolicy = iota
	// WrapHard — break at cols boundary; preserve wide-rune integrity.
	WrapHard
	// WrapNone — single-line truncate; append "…" if overflow (and skip
	// Message.Truncated marker per spec-1.7 R2 D6 priority rule).
	WrapNone
)

// DefaultTheme is the placeholder palette used pre-fixture-lock-in.
// spec-1.7 T-2.5 user-driven CC fixture capture will reveal real CC
// visuals; T-9 R3 fixture verify may errata these constants.
type DefaultTheme struct{}

// Style returns the placeholder style per kind. Uses palette index 8
// (typically "bright black" / "dim grey") for StyleDimmed; truecolor
// RGB for StyleWarning (orange #FFA500) and StyleCodeBg (#282C34).
// MVP — T-9 R3 fixture verify may errata these.
func (DefaultTheme) Style(kind StyleKind) style.Style {
	switch kind {
	case StyleDimmed:
		return style.Style{FG: style.Palette(8)} // palette grey
	case StyleWarning:
		return style.Style{FG: style.RGB(0xFF, 0xA5, 0x00)} // orange
	case StyleCode:
		return style.Style{}
	case StyleCodeBg:
		return style.Style{BG: style.RGB(0x28, 0x2C, 0x34)} // dark grey-blue
	case StyleLangLabel:
		return style.Style{FG: style.Palette(8)} // palette grey
	default:
		return style.Style{}
	}
}

// themeOrDefault returns ctx.Theme if non-nil else DefaultTheme{}.
func themeOrDefault(t StyleTheme) StyleTheme {
	if t == nil {
		return DefaultTheme{}
	}
	return t
}
