// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package input

import (
	"unicode/utf8"

	"github.com/sqlrush/opendbx/internal/app/cli/render/terminal"
)

// ResolveMode applies one keypress to (buffer, cursor) and returns the
// new (buffer, cursor). PURE function — no IO, no side effects, no
// goroutine. spec-1.15 D-2 Update-purity contract.
//
// R3 codex CRIT (import-cycle fix): signature does NOT take
// program.KeyMsg — that would create input → program → input cycle
// since program imports input (DAG 10 imports 9.5). Callers must
// unwrap KeyMsg.Code and KeyMsg.Rune themselves:
//
//	newBuf, newCursor := input.ResolveMode(m.buf, m.cursor, k.Code, k.Rune)
//
// R2 M-6: returns (newBuffer, newCursor) only — Mode is NOT returned.
// Callers derive via input.DeriveMode(newBuffer) at the read site
// (R2 C2 ★A 路径 A by-construction single SoT).
//
// R2 H-7 ★A SCOPE LIMITED: spec-1.16 handles ONLY the cursor==end case
// where cursor equals the rune count of buffer. Callers are responsible
// for maintaining this invariant; mid-cursor edits + KeyDelete are
// deferred to spec-1.17 (❌-10).
//
//   - code == terminal.KeyRune (256): append r to buffer; cursor++ in rune
//     units (1 position regardless of UTF-8 byte width).
//   - code == terminal.KeyBackspace (8): delete the last rune; cursor--;
//     no-op when cursor == 0 (defensive even though caller should not
//     send Backspace on empty buffer).
//   - other code values: returns (buffer, cursor) unchanged.
//
// Cursor unit (R2 M-1): rune position (NOT byte). All inserts advance
// cursor by exactly 1, regardless of UTF-8 byte width of the rune.
func ResolveMode(buffer string, cursor int, code int, r rune) (newBuffer string, newCursor int) {
	switch code {
	case terminal.KeyRune:
		// Append rune; cursor advances by 1 rune position.
		return buffer + string(r), cursor + 1
	case terminal.KeyBackspace:
		if cursor == 0 || buffer == "" {
			return buffer, cursor
		}
		// Strip the last rune (UTF-8 safe via DecodeLastRuneInString).
		_, size := utf8.DecodeLastRuneInString(buffer)
		return buffer[:len(buffer)-size], cursor - 1
	}
	return buffer, cursor
}

// ValueWithoutPrefix strips the mode trigger rune from buffer. Used by
// downstream dispatchers (spec-2.1 slash registry, spec-2.4 SQL parser)
// to extract the post-prefix payload.
//
// R2 M-3 (codex MED-3 fix): does NOT accept a mode parameter — derives
// mode internally via DeriveMode(buffer). UTF-8 safe via utf8.RuneLen
// (defensive against future non-ASCII triggers; current '/' and '\'
// both have RuneLen == 1).
//
// Natural mode and empty buffer both return buffer unchanged.
// A buffer consisting only of the trigger rune returns "".
func ValueWithoutPrefix(buffer string) string {
	mode := DeriveMode(buffer)
	if mode == InputModeNatural || buffer == "" {
		return buffer
	}
	n := utf8.RuneLen(mode.TriggerRune())
	if n < 0 || n > len(buffer) {
		return ""
	}
	return buffer[n:]
}
