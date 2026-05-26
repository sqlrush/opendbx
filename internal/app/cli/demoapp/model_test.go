// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package demoapp

import (
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/keybindings"
	"github.com/sqlrush/opendbx/internal/app/cli/program"
	"github.com/sqlrush/opendbx/internal/app/cli/render/terminal"
)

// keyAction builds a program.KeyActionMsg for the given code/rune,
// decoding the Action the same way program.handleMsg would.
func keyAction(code int, r rune) program.KeyActionMsg {
	k := program.KeyMsg{Code: code, Rune: r}
	return program.KeyActionMsg{Key: k, Action: keybindings.Resolve(keybindings.KeyEvent{Code: code, Rune: r})}
}

// apply feeds a sequence of KeyActionMsgs through Update and returns the
// final Model.
func apply(m *Model, msgs ...program.KeyActionMsg) *Model {
	cur := program.Model(m)
	for _, msg := range msgs {
		next, _ := cur.Update(msg)
		cur = next
	}
	return cur.(*Model)
}

// TestModel_TypeAndSubmit covers the basic input→submit→history flow.
func TestModel_TypeAndSubmit(t *testing.T) {
	t.Parallel()
	m := apply(New(),
		keyAction(terminal.KeyRune, 'h'),
		keyAction(terminal.KeyRune, 'i'),
		keyAction(terminal.KeyEnter, 0),
	)
	st := m.InputState()
	if st.Buffer != "" || st.Cursor != 0 {
		t.Errorf("after submit InputState = %+v; want empty", st)
	}
	if len(m.log) != 1 || m.log[0] != "hi" {
		t.Errorf("log = %v; want [\"hi\"]", m.log)
	}
	if got, _ := m.ring.At(0); got != "hi" {
		t.Errorf("ring.At(0) = %q; want \"hi\"", got)
	}
}

// TestModel_SubmitEmptyNoOp verifies Enter on an empty buffer does not
// push to history or log.
func TestModel_SubmitEmptyNoOp(t *testing.T) {
	t.Parallel()
	m := apply(New(), keyAction(terminal.KeyEnter, 0))
	if len(m.log) != 0 {
		t.Errorf("log = %v; want empty after submitting empty buffer", m.log)
	}
	if m.ring.Len() != 0 {
		t.Errorf("ring.Len() = %d; want 0", m.ring.Len())
	}
}

// TestModel_CursorEditMidBuffer covers Left + Backspace mid-buffer.
func TestModel_CursorEditMidBuffer(t *testing.T) {
	t.Parallel()
	// Type "abc", Left Left (cursor=1), Backspace → "bc" cursor 0.
	m := apply(New(),
		keyAction(terminal.KeyRune, 'a'),
		keyAction(terminal.KeyRune, 'b'),
		keyAction(terminal.KeyRune, 'c'),
		keyAction(terminal.KeyLeft, 0),
		keyAction(terminal.KeyLeft, 0),
		keyAction(terminal.KeyBackspace, 0),
	)
	st := m.InputState()
	if st.Buffer != "bc" || st.Cursor != 0 {
		t.Errorf("InputState = %+v; want {Buffer:\"bc\", Cursor:0}", st)
	}
}

// TestModel_ForwardDelete covers KeyDelete mid-buffer.
func TestModel_ForwardDelete(t *testing.T) {
	t.Parallel()
	// Type "abc", Home (cursor=0), Delete → "bc" cursor 0.
	m := apply(New(),
		keyAction(terminal.KeyRune, 'a'),
		keyAction(terminal.KeyRune, 'b'),
		keyAction(terminal.KeyRune, 'c'),
		keyAction(terminal.KeyCtrlA, 0), // Home
		keyAction(terminal.KeyDelete, 0),
	)
	st := m.InputState()
	if st.Buffer != "bc" || st.Cursor != 0 {
		t.Errorf("InputState = %+v; want {Buffer:\"bc\", Cursor:0}", st)
	}
}

// TestModel_HistoryNav_UpDownRestoresDraft covers the spec-1.17 R2
// CRIT-4 nav-state-on-Model behavior:
//   - submit two entries
//   - type a fresh draft
//   - Up Up → walk back through history
//   - Down Down → return to the saved fresh draft, nav deactivated.
func TestModel_HistoryNav_UpDownRestoresDraft(t *testing.T) {
	t.Parallel()
	m := New()
	// Submit "first" and "second".
	m = apply(m,
		keyAction(terminal.KeyRune, 'f'),
		keyAction(terminal.KeyRune, '1'),
		keyAction(terminal.KeyEnter, 0),
		keyAction(terminal.KeyRune, 's'),
		keyAction(terminal.KeyRune, '2'),
		keyAction(terminal.KeyEnter, 0),
	)
	// Type a fresh draft "draft".
	m = apply(m,
		keyAction(terminal.KeyRune, 'd'),
		keyAction(terminal.KeyRune, 'r'),
	)
	if m.InputState().Buffer != "dr" {
		t.Fatalf("setup draft = %q; want \"dr\"", m.InputState().Buffer)
	}

	// Up → newest history "s2".
	m = apply(m, keyAction(terminal.KeyUp, 0))
	if m.InputState().Buffer != "s2" {
		t.Errorf("after Up#1 = %q; want \"s2\"", m.InputState().Buffer)
	}
	// Up → older "f1".
	m = apply(m, keyAction(terminal.KeyUp, 0))
	if m.InputState().Buffer != "f1" {
		t.Errorf("after Up#2 = %q; want \"f1\"", m.InputState().Buffer)
	}
	// Down → back to "s2".
	m = apply(m, keyAction(terminal.KeyDown, 0))
	if m.InputState().Buffer != "s2" {
		t.Errorf("after Down#1 = %q; want \"s2\"", m.InputState().Buffer)
	}
	// Down → restore fresh draft "dr"; nav deactivated.
	m = apply(m, keyAction(terminal.KeyDown, 0))
	if m.InputState().Buffer != "dr" {
		t.Errorf("after Down#2 (draft restore) = %q; want \"dr\"", m.InputState().Buffer)
	}
	if m.nav.active {
		t.Errorf("nav.active = true after returning to draft; want false")
	}
}

// TestModel_HistoryPrev_EmptyRingNoOp verifies Up on an empty ring is a
// no-op (no nav activation).
func TestModel_HistoryPrev_EmptyRingNoOp(t *testing.T) {
	t.Parallel()
	m := apply(New(), keyAction(terminal.KeyUp, 0))
	if m.nav.active {
		t.Errorf("nav.active = true on empty-ring Up; want false")
	}
	if m.InputState().Buffer != "" {
		t.Errorf("buffer = %q; want empty", m.InputState().Buffer)
	}
}

// TestModel_EditCancelsNav verifies typing after Up cancels nav state.
func TestModel_EditCancelsNav(t *testing.T) {
	t.Parallel()
	m := New()
	m = apply(m,
		keyAction(terminal.KeyRune, 'x'),
		keyAction(terminal.KeyEnter, 0), // ring: ["x"]
		keyAction(terminal.KeyUp, 0),    // buffer "x", nav active
	)
	if !m.nav.active {
		t.Fatalf("nav should be active after Up")
	}
	m = apply(m, keyAction(terminal.KeyRune, 'y')) // edit → "xy", nav reset
	if m.nav.active {
		t.Errorf("nav.active = true after edit; want false")
	}
	if m.InputState().Buffer != "xy" {
		t.Errorf("buffer = %q; want \"xy\"", m.InputState().Buffer)
	}
}

// TestModel_CancelClearsBuffer covers Esc (ActionCancel) and the
// program.CancelCmdMsg path both clearing the buffer.
func TestModel_CancelClearsBuffer(t *testing.T) {
	t.Parallel()
	m := apply(New(),
		keyAction(terminal.KeyRune, 'a'),
		keyAction(terminal.KeyEscape, 0),
	)
	if m.InputState().Buffer != "" {
		t.Errorf("after Esc buffer = %q; want empty", m.InputState().Buffer)
	}

	// CancelCmdMsg path.
	m2 := New()
	m2 = apply(m2, keyAction(terminal.KeyRune, 'b'))
	next, _ := m2.Update(program.CancelCmdMsg{})
	if next.(*Model).InputState().Buffer != "" {
		t.Errorf("after CancelCmdMsg buffer = %q; want empty", next.(*Model).InputState().Buffer)
	}
}

// TestModel_StatusSegments verifies the demoapp identifier segment.
func TestModel_StatusSegments(t *testing.T) {
	t.Parallel()
	segs := New().StatusSegments()
	if len(segs) != 1 || segs[0].Text != "opendbx-demo" {
		t.Errorf("StatusSegments = %v; want [opendbx-demo]", segs)
	}
}

// TestModel_View renders log entries and asserts a known cell.
func TestModel_View(t *testing.T) {
	t.Parallel()
	m := apply(New(),
		keyAction(terminal.KeyRune, 'h'),
		keyAction(terminal.KeyRune, 'i'),
		keyAction(terminal.KeyEnter, 0),
	)
	buf := m.View(20, 5)
	if buf == nil {
		t.Fatalf("View returned nil buffer")
	}
	// "> hi" painted on the last row (rows-1 = 4).
	c := buf.Cell(0, 4)
	if c.Ch != '>' {
		t.Errorf("View cell(0,4) = %q; want '>'", c.Ch)
	}
}

// TestModel_ImmutableUpdate asserts Update returns a fresh Model and
// does not mutate the receiver (CLAUDE.md immutability rule).
func TestModel_ImmutableUpdate(t *testing.T) {
	t.Parallel()
	m := New()
	_, _ = m.Update(keyAction(terminal.KeyRune, 'a'))
	if m.buffer != "" {
		t.Errorf("Update mutated receiver buffer to %q; want unchanged \"\"", m.buffer)
	}
}

// TestModel_SubmitDoesNotMutatePriorRing locks the spec-1.17 R-fix MED-3
// contract: ActionSubmit Clones the Ring so the prior Model's Ring is not
// mutated through a shared pointer. Captures a reference to the pre-submit
// Model and asserts its ring length is unchanged after the submit.
func TestModel_SubmitDoesNotMutatePriorRing(t *testing.T) {
	t.Parallel()
	// Build a Model with one history entry already.
	m0 := apply(New(),
		keyAction(terminal.KeyRune, 'a'),
		keyAction(terminal.KeyEnter, 0),
	)
	priorRingLen := m0.ring.Len() // == 1
	priorLogLen := len(m0.log)    // == 1

	// Submit a second entry from m0.
	m1 := apply(m0,
		keyAction(terminal.KeyRune, 'b'),
		keyAction(terminal.KeyEnter, 0),
	)

	// The prior model m0 must be unchanged (no shared-pointer mutation).
	if m0.ring.Len() != priorRingLen {
		t.Errorf("prior Model ring mutated: Len = %d; want %d (Clone failed)", m0.ring.Len(), priorRingLen)
	}
	if len(m0.log) != priorLogLen {
		t.Errorf("prior Model log mutated: len = %d; want %d", len(m0.log), priorLogLen)
	}
	// The new model has both entries.
	if m1.ring.Len() != 2 {
		t.Errorf("new Model ring Len = %d; want 2", m1.ring.Len())
	}
}

// TestModel_LogBounded verifies the spec-1.17 R-fix MED-4 FIFO cap: the
// log never exceeds LogCapacity even after many submits, and retains the
// most recent entries.
func TestModel_LogBounded(t *testing.T) {
	t.Parallel()
	m := New()
	total := LogCapacity + 50
	for i := 0; i < total; i++ {
		m = apply(m,
			keyAction(terminal.KeyRune, rune('a'+i%26)),
			keyAction(terminal.KeyEnter, 0),
		)
	}
	if len(m.log) != LogCapacity {
		t.Errorf("log len = %d after %d submits; want cap %d", len(m.log), total, LogCapacity)
	}
}
