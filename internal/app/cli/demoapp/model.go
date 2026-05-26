// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package demoapp

import (
	"unicode/utf8"

	"github.com/sqlrush/opendbx/internal/app/cli/input"
	"github.com/sqlrush/opendbx/internal/app/cli/keybindings"
	"github.com/sqlrush/opendbx/internal/app/cli/program"
	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
	"github.com/sqlrush/opendbx/internal/app/cli/render/scheduler"
	"github.com/sqlrush/opendbx/internal/app/cli/render/style"
)

// LogCapacity bounds the demo scrollback log (spec-1.17 R-fix MED-4).
// Without a cap the log grows unbounded across a long session → OOM.
// Aligned with input.RingCapacity. spec-1.20 replaces demoapp with a
// real bounded scrollback.
const LogCapacity = 256

// Model is the spec-1.17 D-6b minimal demonstrator. It carries the
// pipeline state needed to exercise spec-1.15/1.16/1.17:
//
//   - buffer / cursor: drives input.ResolveMode + input.MoveCursor.
//   - ring: history Ring (pure storage; spec-1.17 R2 D-4).
//   - nav: navigation state (active flag, index, draft) — spec-1.17 R2
//     CRIT-4 absorb: lives on Model, NOT on Ring.
//   - log: submitted entries shown in the scrollback area.
//
// Replacement: spec-1.20 LLM client introduces a real Model with the
// same pipeline but real I/O (LLM stream Cmd, scrollback wrapping,
// status indicators).
type Model struct {
	buffer string
	cursor int
	ring   *input.Ring
	nav    historyNav
	log    []string
}

// historyNav is the Up/Down navigation state. Lives on the Model
// (spec-1.17 R2 CRIT-4) so that the Buffer remains the single source
// of truth for *displayed* input — Ring stays pure storage.
type historyNav struct {
	active bool
	index  int // current Ring index; ring.Len() == "back to draft"
	draft  inputState
}

// inputState captures the user's fresh-input draft while navigating
// history. Struct (rather than two parallel string/int fields) so
// spec-1.17.1 multi-row can extend with Lines []string without
// breaking callers (spec-1.17 R2 R-9 mitigation).
type inputState struct {
	Buffer string
	Cursor int
}

// New constructs an empty demoapp Model.
func New() *Model {
	return &Model{
		ring: input.NewRing(),
	}
}

// Init runs nothing yet — demoapp has no startup Cmd.
func (m *Model) Init() scheduler.Cmd { return nil }

// Update applies a Msg to the Model and returns the new Model + any
// Cmd. Pure function (spec-1.15 D-2): returns a *fresh* Model on every
// key event (immutable update per CLAUDE.md global coding style).
//
// Recognized Msg types:
//   - program.KeyActionMsg — primary input path; switches on .Action.
//   - program.CancelCmdMsg — Esc / Ctrl+C first press; resets buffer.
//   - program.ResizeMsg / QuitMsg etc. — no-op (handled by program).
//
// Unknown Msg types pass through with no state change.
func (m *Model) Update(msg scheduler.Msg) (program.Model, scheduler.Cmd) {
	switch v := msg.(type) {
	case program.KeyActionMsg:
		return m.handleAction(v), nil
	case program.CancelCmdMsg:
		// Spec-1.15 D-5 first-press Cancel: clear buffer + nav state.
		next := *m
		next.buffer = ""
		next.cursor = 0
		next.nav = historyNav{}
		return &next, nil
	}
	return m, nil
}

// handleAction is the per-Action state-transition table.
func (m *Model) handleAction(msg program.KeyActionMsg) *Model {
	next := *m
	switch msg.Action {
	case keybindings.ActionInsertRune:
		next.buffer, next.cursor = input.ResolveMode(m.buffer, m.cursor, msg.Key.Code, msg.Key.Rune)
		next.nav = historyNav{} // any edit cancels nav

	case keybindings.ActionDeleteBackward, keybindings.ActionDeleteForward:
		next.buffer, next.cursor = input.ResolveMode(m.buffer, m.cursor, msg.Key.Code, msg.Key.Rune)
		next.nav = historyNav{}

	case keybindings.ActionMoveLeft, keybindings.ActionMoveRight,
		keybindings.ActionMoveHome, keybindings.ActionMoveEnd:
		next.cursor = input.MoveCursor(m.buffer, m.cursor, program.ActionToMovement(msg.Action))

	case keybindings.ActionHistoryPrev:
		applyHistoryPrev(&next)

	case keybindings.ActionHistoryNext:
		applyHistoryNext(&next)

	case keybindings.ActionSubmit:
		if m.buffer != "" {
			// spec-1.17 R-fix MED-3: Clone the Ring so the prior Model's
			// Ring is not mutated through a shared pointer (immutable update).
			next.ring = m.ring.Clone()
			next.ring.Push(m.buffer)
			next.log = appendLogBounded(m.log, m.buffer)
		}
		next.buffer = ""
		next.cursor = 0
		next.nav = historyNav{}

	case keybindings.ActionCancel:
		// Esc — clear buffer (similar to CancelCmdMsg).
		next.buffer = ""
		next.cursor = 0
		next.nav = historyNav{}

	case keybindings.ActionNone, keybindings.ActionQuit:
		// ActionNone: unknown key; ignore. ActionQuit: short-circuited by
		// program.preDispatchSystem so demoapp never sees this in practice.
	}
	return &next
}

// applyHistoryPrev moves cursor backward through ring history, saving
// the user's draft on the first Up press.
func applyHistoryPrev(m *Model) {
	if m.ring.Len() == 0 {
		return
	}
	if !m.nav.active {
		m.nav.active = true
		m.nav.draft = inputState{Buffer: m.buffer, Cursor: m.cursor}
		m.nav.index = m.ring.Len() // pointing at fresh draft slot
	}
	if m.nav.index > 0 {
		m.nav.index--
		entry, _ := m.ring.At(m.nav.index)
		m.buffer = entry
		m.cursor = utf8.RuneCountInString(entry)
	}
}

// applyHistoryNext moves cursor forward; reaching the draft slot
// restores the saved fresh-input and clears nav.active.
func applyHistoryNext(m *Model) {
	if !m.nav.active {
		return
	}
	m.nav.index++
	if m.nav.index >= m.ring.Len() {
		m.buffer = m.nav.draft.Buffer
		m.cursor = m.nav.draft.Cursor
		m.nav = historyNav{}
		return
	}
	entry, _ := m.ring.At(m.nav.index)
	m.buffer = entry
	m.cursor = utf8.RuneCountInString(entry)
}

// appendLogBounded appends entry to log, evicting the oldest entries
// when the length would exceed LogCapacity (spec-1.17 R-fix MED-4 FIFO
// bound). Returns a fresh slice — does not mutate the input (immutable
// update; the prior Model keeps its log).
func appendLogBounded(log []string, entry string) []string {
	next := make([]string, 0, len(log)+1)
	next = append(next, log...)
	next = append(next, entry)
	if len(next) > LogCapacity {
		next = next[len(next)-LogCapacity:]
	}
	return next
}

// View renders the log entries as the scrollback area. Each log line
// is painted left-aligned with the default style; rows are filled
// bottom-up so the newest entry sits just above the input row.
func (m *Model) View(cols, rows int) buffer.Buffer {
	g, err := buffer.NewGrid(cols, rows)
	if err != nil {
		return nil
	}
	// Paint log entries bottom-up.
	for i := 0; i < rows && i < len(m.log); i++ {
		row := rows - 1 - i
		entry := m.log[len(m.log)-1-i]
		paintText(g, "> "+entry, 0, row, style.Style{})
	}
	return g
}

// InputState exposes the current buffer + cursor to program.renderFn
// (which paints the input row above the status line; see spec-1.16
// D-5 / paintInputRow).
func (m *Model) InputState() program.InputState {
	return program.InputState{Buffer: m.buffer, Cursor: m.cursor}
}

// StatusSegments adds a demoapp identifier so users see this is the
// minimal demonstrator (not yet spec-1.20 LLM client).
func (m *Model) StatusSegments() []program.StatusSegment {
	return []program.StatusSegment{
		{Text: "opendbx-demo"},
	}
}

// LogForTest returns a copy of the submitted-entry log. Test seam for
// the spec-1.17 D-7 integration harness (cross-package observation of
// the immutable Model state after the program loop drains).
func (m *Model) LogForTest() []string {
	out := make([]string, len(m.log))
	copy(out, m.log)
	return out
}

// LogContainsForTest reports whether entry appears in the log. Test
// seam convenience for the integration harness.
func (m *Model) LogContainsForTest(entry string) bool {
	for _, e := range m.log {
		if e == entry {
			return true
		}
	}
	return false
}

// paintText writes runes left-aligned into grid; stops at cols.
func paintText(g *buffer.Grid, s string, x, row int, st style.Style) {
	cols, _ := g.Size()
	for _, r := range s {
		if x >= cols {
			break
		}
		g.SetCell(x, row, buffer.Cell{Ch: r, St: st})
		x++
	}
}
