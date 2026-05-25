// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package keybindings

import (
	"github.com/sqlrush/opendbx/internal/app/cli/render/terminal"
)

// Action is the high-level intent decoded from a KeyEvent by Resolve.
// Models receive Action via program.KeyActionMsg and switch on it.
//
// ── Invariant — Action append-only (spec-1.17 R2 HIGH-5) ──────────────
// Reorder / delete on any existing const is BREAKING because
// KeyActionMsg.Action is wire-serialized across spec boundaries
// (program → demoapp → spec-1.20 LLM client). spec-2.1 ActionTab /
// spec-2.4 ActionComplete / spec-2.x ActionVimMode etc. MUST append
// to the end of this list, never insert.
type Action int

const (
	// ActionNone is the zero value — unhandled key. Caller falls
	// through to raw KeyMsg dispatch.
	ActionNone Action = iota
	// ActionInsertRune — KeyRune event; rune carried in KeyEvent.Rune.
	ActionInsertRune
	// ActionDeleteBackward — Backspace; delete rune before cursor.
	ActionDeleteBackward
	// ActionDeleteForward — Delete (271); delete rune at cursor.
	ActionDeleteForward
	// ActionMoveLeft — Left arrow; cursor -= 1 rune.
	ActionMoveLeft
	// ActionMoveRight — Right arrow; cursor += 1 rune.
	ActionMoveRight
	// ActionMoveHome — Ctrl+A; cursor → 0.
	ActionMoveHome
	// ActionMoveEnd — Ctrl+E; cursor → rune_count(buffer).
	ActionMoveEnd
	// ActionHistoryPrev — Up arrow; navigate history backward.
	ActionHistoryPrev
	// ActionHistoryNext — Down arrow; navigate history forward.
	ActionHistoryNext
	// ActionSubmit — Enter; submit current input.
	ActionSubmit
	// ActionCancel — Esc; cancel current input / inflight command.
	ActionCancel
	// ActionQuit — Ctrl+C; quit protocol. Reserved (spec-1.15 D-5
	// double-press); program.preDispatchSystem short-circuits.
	ActionQuit
)

// String renders an Action for debug / logging. Returns the const
// identifier minus the "Action" prefix; e.g. "InsertRune", "Quit".
// Unknown values return "Action(<n>)".
func (a Action) String() string {
	switch a {
	case ActionNone:
		return "None"
	case ActionInsertRune:
		return "InsertRune"
	case ActionDeleteBackward:
		return "DeleteBackward"
	case ActionDeleteForward:
		return "DeleteForward"
	case ActionMoveLeft:
		return "MoveLeft"
	case ActionMoveRight:
		return "MoveRight"
	case ActionMoveHome:
		return "MoveHome"
	case ActionMoveEnd:
		return "MoveEnd"
	case ActionHistoryPrev:
		return "HistoryPrev"
	case ActionHistoryNext:
		return "HistoryNext"
	case ActionSubmit:
		return "Submit"
	case ActionCancel:
		return "Cancel"
	case ActionQuit:
		return "Quit"
	}
	return "Action(?)"
}

// KeyEvent is the input-side dehydrated key event passed to Resolve.
// Field shape is a thin re-export of terminal.KeyMsg / program.KeyMsg
// (R2 invariant — Code int SoT lives in render/terminal/driver.go;
// reorder / drift across this struct vs program.KeyMsg is BREAKING).
type KeyEvent struct {
	Code int  // terminal.Key* value (terminal.KeyRune / KeyBackspace / etc.)
	Rune rune // valid when Code == terminal.KeyRune; informational otherwise
	Mod  uint8
}

// Resolve maps a KeyEvent to a high-level Action via DefaultBindings.
// Returns ActionNone for codes not in the binding table — caller
// (program.handleMsg) falls through to raw KeyMsg dispatch so Models
// implementing custom key handling can still intercept.
//
// spec-2.x user customization will overlay LoadUserBindings on top of
// DefaultBindings; Resolve will consult the merged table.
func Resolve(event KeyEvent) Action {
	if event.Code == terminal.KeyRune {
		// Rune events always insert; modifier-laden rune events (Alt+x,
		// Ctrl+x where x is printable) are deferred — spec-2.x will
		// add bindings keyed on (Code, Mod) tuples.
		return ActionInsertRune
	}
	if a, ok := defaultBindings[event.Code]; ok {
		return a
	}
	return ActionNone
}

// IsReserved reports whether an Action is hard-reserved (not allowed to
// be rebound by spec-2.x user customization). spec-1.17 R2 (MED-2):
// minimal set = {ActionQuit}. CC also reserves Ctrl+D EOF / Ctrl+M
// return; opendbx defers those to spec-2.x reservedShortcuts overlay.
func IsReserved(a Action) bool {
	return a == ActionQuit
}

// defaultBindings is the spec-1.17 R2 default key → Action table.
// keyed on terminal.Key* int code (NOT including KeyRune — Resolve
// handles that explicitly above).
//
// Ordering note: lowest-value codes first for readability; the map
// access in Resolve is O(1) so order is presentation-only.
var defaultBindings = map[int]Action{
	terminal.KeyBackspace:     ActionDeleteBackward, // 8
	terminal.KeyEnter:         ActionSubmit,         // 13
	terminal.KeyEscape:        ActionCancel,         // 27
	terminal.KeyCtrlA:         ActionMoveHome,       // 65
	terminal.KeyCtrlC:         ActionQuit,           // 67 — reserved (preDispatchSystem)
	terminal.KeyCtrlE:         ActionMoveEnd,        // 69
	terminal.KeyCtrlBackslash: ActionQuit,           // 92 — hard exit alias (spec-0.12)
	terminal.KeyUp:            ActionHistoryPrev,    // 257
	terminal.KeyDown:          ActionHistoryNext,    // 258
	terminal.KeyRight:         ActionMoveRight,      // 259
	terminal.KeyLeft:          ActionMoveLeft,       // 260
	terminal.KeyDelete:        ActionDeleteForward,  // 271
}
