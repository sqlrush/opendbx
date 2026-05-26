// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package input

import (
	"fmt"
	"unicode/utf8"
)

// MovementKind is the cursor movement intent passed to MoveCursor
// (spec-1.17 R2 D-3 / CRIT-2 absorb). Lives in input package — NOT
// keybindings — to keep the input(9.5) → keybindings(9.6) DAG direction
// forward only. program (10) translates keybindings.Action to
// input.MovementKind via program.ActionToMovement at the dispatch boundary.
type MovementKind int

const (
	// MoveNone is the zero value; MoveCursor returns cursor unchanged.
	MoveNone MovementKind = iota
	// MoveLeft moves cursor one rune toward buffer start (clamped to 0).
	MoveLeft
	// MoveRight moves cursor one rune toward buffer end (clamped to rune count).
	MoveRight
	// MoveHome jumps cursor to 0.
	MoveHome
	// MoveEnd jumps cursor to rune_count(buffer).
	MoveEnd
)

// String renders a MovementKind for debug / logging.
func (m MovementKind) String() string {
	switch m {
	case MoveNone:
		return "None"
	case MoveLeft:
		return "Left"
	case MoveRight:
		return "Right"
	case MoveHome:
		return "Home"
	case MoveEnd:
		return "End"
	}
	return fmt.Sprintf("MovementKind(%d)", int(m))
}

// MoveCursor returns the new cursor position after applying movement
// to (buffer, cursor). Buffer is unchanged; pure function.
//
// Invariant: 0 ≤ result ≤ rune_count(buffer); cursor passed in is
// clamped to that range first (MED-9 defensive). MoveNone returns
// the clamped cursor unchanged.
//
// spec-1.17 single-line only: MoveLeft / MoveRight step by 1 rune;
// MoveHome / MoveEnd jump to line bounds. spec-1.17.1 multi-row will
// extend with word-wise (Alt+B/F) + row-jump (Up/Down inside multiline
// buffer) movements.
func MoveCursor(buffer string, cursor int, movement MovementKind) int {
	runeCount := utf8.RuneCountInString(buffer)
	// MED-9 clamp.
	if cursor < 0 {
		cursor = 0
	}
	if cursor > runeCount {
		cursor = runeCount
	}

	switch movement {
	case MoveLeft:
		if cursor > 0 {
			return cursor - 1
		}
		return 0
	case MoveRight:
		if cursor < runeCount {
			return cursor + 1
		}
		return runeCount
	case MoveHome:
		return 0
	case MoveEnd:
		return runeCount
	}
	return cursor
}
