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
	// StyleBold — heading + **strong** emphasis (spec-1.11 D-3).
	StyleBold
	// StyleItalic — *italic* emphasis (spec-1.11 D-3).
	StyleItalic
	// StyleLink — link visible text + dim URL fallback (spec-1.11 D-3 ❌-8).
	StyleLink
	// StyleHeading — heading depth styling (CC formatToken permission-colored;
	// spec-1.11 D-3 R3 baseline — no prefix glyph).
	StyleHeading
	// StyleKeyword — code keyword token (spec-1.12 D-2 Q4 ★A coarse-8).
	StyleKeyword
	// StyleString — code string literal (spec-1.12 D-2 Q4 ★A coarse-8).
	StyleString
	// StyleNumber — code numeric literal (spec-1.12 D-2 Q4 ★A coarse-8).
	StyleNumber
	// StyleComment — code comment (spec-1.12 D-2 Q4 ★A coarse-8).
	StyleComment
	// StyleName — code identifier/name (spec-1.12 D-2 Q4 ★A coarse-8).
	StyleName
	// StyleOperator — code operator token (spec-1.12 D-2 Q4 ★A coarse-8).
	StyleOperator
	// StylePunctuation — code punctuation (spec-1.12 D-2 Q4 ★A coarse-8).
	StylePunctuation
)

// StyleTheme provides the palette resolution from semantic StyleKind to
// concrete style.Style. Implementations: DefaultTheme (MVP); future
// LightTheme / DarkTheme / UserCustomTheme (spec-1.x runtime switch).
//
// Implementations with instance-level state (e.g., user-customized
// palette per session) MUST implement [blockKeyer] (defined below)
// to prevent stale cached buffers in the spec-1.11 Markdown LRU cache.
// Stateless themes (e.g., DefaultTheme) rely on the type-name fallback
// in themeCacheKey (R6 NIT-2 co-locate).
type StyleTheme interface {
	Style(kind StyleKind) style.Style
}

// blockKeyer is the optional opt-in interface a StyleTheme may
// implement to control its cache key identity in the block LRU buffer
// cache (used by spec-1.11 Markdown + spec-1.12 Code).
//
// spec-1.11 § 3.3 R3.1 Step 2 introduced this as markdownKeyer; spec-1.12
// R2 MED-1 renamed to blockKeyer (and MarkdownCacheKey → BlockCacheKey)
// reflecting cache scope generalization (markdownCache → blockCache).
//
// Stateless StyleTheme implementations need NOT implement this — the
// type-name fallback (`%T`) in themeCacheKey suffices. Stateful themes
// (e.g., palette swap on runtime theme switch in spec-3.x) MUST
// implement it; otherwise same-type/different-state themes silently
// share stale cached buffers.
type blockKeyer interface {
	BlockCacheKey() string
}

// HighlighterTheme is the opt-in interface a StyleTheme may implement
// to drive spec-1.12 chroma syntax highlighting. Themes without this
// interface fall through to plain monospace rendering (graceful
// degradation; spec-1.12 § 2.3 I-3).
//
// spec-1.12 R2 CRIT-2 ★A: SupportsTrueColor() removed — block layer
// cannot import tcell (IMP-9 isolation). Color depth comes from
// ctx.ColorDepth injected by spec-1.15 TUI / caller.
type HighlighterTheme interface {
	// CodeStyle returns the chroma style name (e.g., "monokai",
	// "github", "dracula"). Must be a registered name in
	// github.com/alecthomas/chroma/v2/styles. Return "" or unregistered
	// name → falls back to "monokai" (opendbx dark-default
	// approximation per spec-1.12 D-5 R3 B-34 — not exact CC parity).
	CodeStyle() string
}

// CodeStyle satisfies HighlighterTheme. spec-1.12 R2 CRIT-1 ★B:
// DefaultTheme opts in to highlighting by default (default-on highlight,
// matching CC Markdown fence behavior). Returns "monokai" as a Go-port
// dark-default approximation (not a claim of CC exact theme parity).
func (DefaultTheme) CodeStyle() string { return "monokai" }

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
	case StyleBold:
		return style.Style{Bold: true}
	case StyleItalic:
		return style.Style{Italic: true}
	case StyleLink:
		return style.Style{FG: style.Palette(8), Underline: true} // dim + underline
	case StyleHeading:
		return style.Style{Bold: true} // permission-colored TBD post T-9 fixture
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
