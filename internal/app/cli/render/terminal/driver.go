// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package terminal

import (
	"context"

	"github.com/sqlrush/opendbx/internal/app/cli/render/style"
)

// Event is the marker interface for events delivered by Driver.PollEvent.
// Concrete types: EventKey / EventResize / EventInterrupt.
//
// tcell-free contract: Driver implementations convert tcell.Event into
// the render-subsystem Event types so callers never see tcell types.
type Event interface{ isEvent() }

// Keypress key codes (tcell-free). 0..31 are control characters; 32+
// are printable runes; higher values map opendbx-specific virtual keys.
const (
	KeyNone          = 0
	KeyCtrlC         = 3  // ETX
	KeyEnter         = 13 // CR
	KeyCtrlBackslash = 28
	KeyEscape        = 27
	KeyRune          = 256 // Use the Rune field instead of Code
)

// Modifier bit flags packed into Event.ShiftCtrlAlt uint8.
const (
	ModShift = 1 << 0
	ModCtrl  = 1 << 1
	ModAlt   = 1 << 2
)

// EventKey is a keypress event. Code identifies the key (KeyCtrlC etc.).
// When Code == KeyRune the Rune field carries the printable character.
type EventKey struct {
	Code         int
	Rune         rune
	ShiftCtrlAlt uint8
}

func (EventKey) isEvent() {}

// EventResize is delivered when the terminal window size changes
// (SIGWINCH on Unix / tcell event on other platforms).
type EventResize struct {
	Cols, Rows int
}

func (EventResize) isEvent() {}

// EventInterrupt is delivered via Driver.PostEvent and carries an
// opaque payload (e.g. context-cancel signal).
type EventInterrupt struct {
	Data any
}

func (EventInterrupt) isEvent() {}

// HasMod reports whether the modifier bit is set on a key event.
// Helper for callers checking Ctrl/Shift/Alt without bit-manipulation.
func (ek EventKey) HasMod(mod uint8) bool {
	return ek.ShiftCtrlAlt&mod != 0
}

// ModifierString returns a human-readable list of active modifiers
// ("Ctrl+Shift+Alt"); empty string if no modifiers set. Useful for
// debug logging and test assertions.
func (ek EventKey) ModifierString() string {
	var parts []string
	if ek.HasMod(ModCtrl) {
		parts = append(parts, "Ctrl")
	}
	if ek.HasMod(ModShift) {
		parts = append(parts, "Shift")
	}
	if ek.HasMod(ModAlt) {
		parts = append(parts, "Alt")
	}
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for _, p := range parts[1:] {
		out += "+" + p
	}
	return out
}

// Driver is the render-subsystem terminal abstraction. Implementations
// adapt tcell.Screen / SimulationScreen / future backends.
//
// Lifecycle: Init → ... → Fini. Show flushes the current frame; Sync
// re-emits the full frame (used after resize). PollEvent honors the
// passed context: if ctx is cancelled before an event arrives the
// method MUST return ctx.Err() (spec-0.12 PollEvent blocking lesson
// — Driver implementations bridge ctx.Done via PostEvent internally).
//
// PostEvent error contract (spec-0.13 T-13 code-reviewer R1 MED-2): the
// returned error indicates the driver rejected the event — either the
// driver is post-Fini or the internal event queue is at capacity. spec-1.4
// driver impls MUST document the exact backpressure policy.
type Driver interface {
	Init() error
	Fini()
	Show()
	Sync()
	Clear()
	Size() (cols, rows int)
	SetCell(x, y int, ch rune, st style.Style)
	PollEvent(ctx context.Context) (Event, error)
	PostEvent(ev Event) error
	Resize(cols, rows int)
}
