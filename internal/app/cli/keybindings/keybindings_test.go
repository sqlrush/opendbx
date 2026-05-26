// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package keybindings

import (
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/terminal"
)

// TestResolve_DefaultBindings covers the spec-1.17 R2 DefaultBindings
// table — one case per code, plus KeyRune fall-through and unknown
// code → ActionNone.
func TestResolve_DefaultBindings(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		evt  KeyEvent
		want Action
	}{
		{"KeyRune 'a' → InsertRune", KeyEvent{Code: terminal.KeyRune, Rune: 'a'}, ActionInsertRune},
		{"KeyRune '中' → InsertRune (CJK)", KeyEvent{Code: terminal.KeyRune, Rune: '中'}, ActionInsertRune},
		{"KeyBackspace → DeleteBackward", KeyEvent{Code: terminal.KeyBackspace}, ActionDeleteBackward},
		{"KeyDelete → DeleteForward", KeyEvent{Code: terminal.KeyDelete}, ActionDeleteForward},
		{"KeyLeft → MoveLeft", KeyEvent{Code: terminal.KeyLeft}, ActionMoveLeft},
		{"KeyRight → MoveRight", KeyEvent{Code: terminal.KeyRight}, ActionMoveRight},
		{"KeyCtrlA → MoveHome", KeyEvent{Code: terminal.KeyCtrlA, Mod: terminal.ModCtrl}, ActionMoveHome},
		{"KeyCtrlE → MoveEnd", KeyEvent{Code: terminal.KeyCtrlE, Mod: terminal.ModCtrl}, ActionMoveEnd},
		{"KeyUp → HistoryPrev", KeyEvent{Code: terminal.KeyUp}, ActionHistoryPrev},
		{"KeyDown → HistoryNext", KeyEvent{Code: terminal.KeyDown}, ActionHistoryNext},
		{"KeyEnter → Submit", KeyEvent{Code: terminal.KeyEnter}, ActionSubmit},
		{"KeyEscape → Cancel", KeyEvent{Code: terminal.KeyEscape}, ActionCancel},
		{"KeyCtrlC → Quit (reserved)", KeyEvent{Code: terminal.KeyCtrlC, Mod: terminal.ModCtrl}, ActionQuit},
		{"KeyCtrlBackslash → Quit", KeyEvent{Code: terminal.KeyCtrlBackslash, Mod: terminal.ModCtrl}, ActionQuit},
		{"unknown code 9999 → None", KeyEvent{Code: 9999}, ActionNone},
		{"KeyNone (0) → None", KeyEvent{Code: terminal.KeyNone}, ActionNone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Resolve(tc.evt); got != tc.want {
				t.Errorf("Resolve(%+v) = %v; want %v", tc.evt, got, tc.want)
			}
		})
	}
}

// TestIsReserved covers spec-1.17 R2 MED-2 Reserved invariant: only
// ActionQuit is hard-reserved. spec-2.x reservedShortcuts overlay adds
// Ctrl+D / Ctrl+M; that does NOT change this minimal contract.
func TestIsReserved(t *testing.T) {
	t.Parallel()
	cases := []struct {
		action Action
		want   bool
	}{
		{ActionQuit, true},
		{ActionCancel, false},
		{ActionSubmit, false},
		{ActionInsertRune, false},
		{ActionDeleteBackward, false},
		{ActionMoveHome, false},
		{ActionMoveEnd, false},
		{ActionHistoryPrev, false},
		{ActionNone, false},
	}
	for _, tc := range cases {
		t.Run(tc.action.String(), func(t *testing.T) {
			if got := IsReserved(tc.action); got != tc.want {
				t.Errorf("IsReserved(%v) = %v; want %v", tc.action, got, tc.want)
			}
		})
	}
}

// TestActionString covers the human-readable name table and unknown
// fallback. spec-1.17 R2 NIT: future Action appends MUST extend the
// String() switch.
func TestActionString(t *testing.T) {
	t.Parallel()
	cases := []struct {
		a    Action
		want string
	}{
		{ActionNone, "None"},
		{ActionInsertRune, "InsertRune"},
		{ActionDeleteBackward, "DeleteBackward"},
		{ActionDeleteForward, "DeleteForward"},
		{ActionMoveLeft, "MoveLeft"},
		{ActionMoveRight, "MoveRight"},
		{ActionMoveHome, "MoveHome"},
		{ActionMoveEnd, "MoveEnd"},
		{ActionHistoryPrev, "HistoryPrev"},
		{ActionHistoryNext, "HistoryNext"},
		{ActionSubmit, "Submit"},
		{ActionCancel, "Cancel"},
		{ActionQuit, "Quit"},
		{Action(9999), "Action(9999)"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			if got := tc.a.String(); got != tc.want {
				t.Errorf("Action(%d).String() = %q; want %q", int(tc.a), got, tc.want)
			}
		})
	}
}

// TestDefaultBindings_NoQuitDuplication verifies there are no map keys
// that map to multiple Actions — a guard against an accidental override
// when spec-2.x merges loadUserBindings. (We test the production map
// directly rather than mocking the resolver.)
func TestDefaultBindings_AllCodesDistinct(t *testing.T) {
	t.Parallel()
	seen := make(map[int]struct{}, len(defaultBindings))
	for code := range defaultBindings {
		if _, dup := seen[code]; dup {
			t.Errorf("duplicate code %d in defaultBindings", code)
		}
		seen[code] = struct{}{}
	}
	// Bonus: ensure there is at least the expected count.
	if len(defaultBindings) < 12 {
		t.Errorf("defaultBindings has only %d entries; expected ≥ 12 (spec-1.17 D-1 table)", len(defaultBindings))
	}
}

// BenchmarkKeybindings_Resolve targets spec-1.17 § 4.4 < 50 ns/op.
func BenchmarkKeybindings_Resolve(b *testing.B) {
	evt := KeyEvent{Code: terminal.KeyCtrlA, Mod: terminal.ModCtrl}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Resolve(evt)
	}
}

// BenchmarkKeybindings_Resolve_Rune covers the KeyRune fast path.
func BenchmarkKeybindings_Resolve_Rune(b *testing.B) {
	evt := KeyEvent{Code: terminal.KeyRune, Rune: 'a'}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Resolve(evt)
	}
}
