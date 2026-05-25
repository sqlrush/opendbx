// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package input

// Mode is the spec-1.16 three-mode tag, classified from Buffer[0].
//
// R2 C2 ★A 路径 A: Mode is NEVER stored as state — always derived
// via DeriveMode(buffer) at the read site. This makes the invariant
// "mode is consistent with Buffer" trivially provable by construction
// (single source of truth = Buffer).
type Mode int

const (
	// ModeNatural is the default mode; any non-trigger first character
	// (or empty buffer) puts the input in natural-language mode. Downstream
	// (spec-1.20 llm-client) treats this buffer as LLM prompt.
	ModeNatural Mode = iota

	// ModeSlash triggers on '/' as the first character. Downstream
	// (spec-2.1 slash-registry) consumes ValueWithoutPrefix(buffer) for
	// command dispatch.
	ModeSlash

	// ModeSQL triggers on '\' as the first character (psql parity:
	// '\d', '\dt', etc.; opendb historical identity). Downstream
	// (spec-2.4 sql-parse) consumes ValueWithoutPrefix(buffer) for SQL
	// execution.
	ModeSQL
)

// String returns a human-readable mode name for status line / debug.
// Stable identifiers: "natural" / "slash" / "sql".
func (m Mode) String() string {
	switch m {
	case ModeNatural:
		return "natural"
	case ModeSlash:
		return "slash"
	case ModeSQL:
		return "sql"
	default:
		return "natural"
	}
}

// TriggerRune returns the character that activates this mode when
// placed at Buffer[0]. Returns NUL (rune 0) for Natural — natural mode
// has no trigger character (any non-trigger first byte yields Natural).
func (m Mode) TriggerRune() rune {
	switch m {
	case ModeSlash:
		return '/'
	case ModeSQL:
		return '\\'
	default:
		return 0
	}
}

// DeriveMode is the SOLE source of truth for mode classification.
//
// Mode is derived from buffer[0] (the first byte). Both trigger
// characters '/' (0x2F) and '\' (0x5C) are single-byte ASCII so byte
// indexing is equivalent to rune indexing for this comparison (spec-
// 1.16 R2 H-2 explicit ASCII-safe note). Empty buffer is Natural by
// definition.
//
// R2 C2 ★A 路径 A: mode is NEVER stored as a field; callers invoke
// DeriveMode at read site. Buffer is the single SoT — invariant by-
// construction.
func DeriveMode(buffer string) Mode {
	if buffer == "" {
		return ModeNatural
	}
	switch buffer[0] {
	case '/':
		return ModeSlash
	case '\\':
		return ModeSQL
	default:
		return ModeNatural
	}
}
