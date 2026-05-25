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
// spec-1.17 R2 D-2: cursor ANY position is supported (spec-1.16 H-7
// scope-limit cursor==end is RETIRED; old callers still work because
// cursor==rune_count is a special case of "any"). KeyDelete (271)
// forward-delete added. cursor clamped to [0, rune_count(buffer)] on
// input (defensive — caller bug → no panic, MED-9).
//
//   - code == terminal.KeyRune: insert r at rune position cursor;
//     cursor advances by 1 rune position.
//   - code == terminal.KeyBackspace (8): delete rune at cursor-1;
//     cursor--; no-op when cursor == 0.
//   - code == terminal.KeyDelete (271, spec-1.17 R2 NEW): delete rune
//     at cursor (forward); cursor unchanged; no-op when cursor ==
//     rune_count(buffer).
//   - other code values: returns (buffer, cursor) unchanged.
//
// Cursor unit (R2 M-1): rune position (NOT byte). All inserts advance
// cursor by exactly 1, regardless of UTF-8 byte width of the rune.
func ResolveMode(buffer string, cursor int, code int, r rune) (newBuffer string, newCursor int) {
	runes := []rune(buffer)
	// MED-9 clamp invariant: defensive clamp for caller bugs.
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}

	switch code {
	case terminal.KeyRune:
		// Insert rune at rune position cursor; cursor advances by 1.
		// (cursor==len(runes) → trivial append; spec-1.16 H-7 forward-compat.)
		next := make([]rune, 0, len(runes)+1)
		next = append(next, runes[:cursor]...)
		next = append(next, r)
		next = append(next, runes[cursor:]...)
		return string(next), cursor + 1
	case terminal.KeyBackspace:
		if cursor == 0 || len(runes) == 0 {
			return buffer, cursor
		}
		// Delete rune at cursor-1.
		next := make([]rune, 0, len(runes)-1)
		next = append(next, runes[:cursor-1]...)
		next = append(next, runes[cursor:]...)
		return string(next), cursor - 1
	case terminal.KeyDelete:
		// spec-1.17 R2 D-2 NEW: forward delete at cursor.
		if cursor >= len(runes) || len(runes) == 0 {
			return buffer, cursor
		}
		next := make([]rune, 0, len(runes)-1)
		next = append(next, runes[:cursor]...)
		next = append(next, runes[cursor+1:]...)
		return string(next), cursor
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
	if mode == ModeNatural || buffer == "" {
		return buffer
	}
	n := utf8.RuneLen(mode.TriggerRune())
	if n < 0 || n > len(buffer) {
		return ""
	}
	return buffer[n:]
}
