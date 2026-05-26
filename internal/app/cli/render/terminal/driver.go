// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package terminal

import (
	"context"

	"github.com/gdamore/tcell/v2"

	"github.com/sqlrush/opendbx/internal/app/cli/render/style"
)

// Event is the marker interface for events delivered by Driver.PollEvent.
// Concrete types: EventKey / EventResize / EventInterrupt.
//
// tcell-free CALLER contract: Driver implementations convert tcell.Event into
// the render-subsystem Event types so callers never see tcell types. The
// terminal package itself aliases tcell.Key* constants for Code int values
// (spec-1.17 R2 D-1 / CRIT-1) to keep tcell as the authoritative source of
// truth — this avoids hardcoded ASCII drift (spec-1.17 R1 had 5/7 wrong
// values). Importing tcell here is permitted by IMP-9 whitelist (4 pkgs:
// render/terminal + tui + bootstrap + platform/terminal).
type Event interface{ isEvent() }

// Keypress key codes. Aliased to tcell.Key* iota values (spec-1.17 R2
// D-1 / CRIT-1) so tcell version upgrades automatically propagate AND
// the values match what the render/terminal/tcell adapter emits from
// tcell's input layer (which normalizes raw control bytes via
// KeyCtrlSpace+iota+64, NOT raw ASCII; see tcell input.go:451-452).
//
// Stable shared values: KeyBackspace=8 / KeyEnter=13 / KeyEscape=27 all
// match tcell aliases (KeyBS/KeyCR/KeyESC) and ASCII at the same time.
// KeyRune=256 is a stable sentinel for "Rune field carries the char".
//
// spec-1.15 source notes: KeyCtrlC moved from ASCII ETX=3 to
// int(tcell.KeyCtrlC)=67 in spec-1.17. spec-1.15 production never wired
// a real tcell adapter (Q13 ★A retroactive impl deferred to spec-1.17
// D-6b), so the change is invisible to consumers that reference the
// named constant; only literal 3 / 28 comparisons would have to change
// (none exist in tree).
const (
	KeyNone      = 0
	KeyBackspace = 8  // BS (tcell.KeyBackspace alias to KeyBS=8; spec-1.16 R2 C1 input editing)
	KeyEnter     = 13 // CR (tcell.KeyEnter alias to KeyCR=13)
	KeyEscape    = 27 // ESC (tcell.KeyEscape alias to KeyESC=27)
	KeyRune      = 256

	// spec-1.17 R2 H-1: tcell alias constants. Values from tcell v2.13.9
	// key.go (KeyCtrlSpace+iota+64 for Ctrl* family; explicit iota for
	// arrow / Delete). tcell.KeyBackspace2 (127, ASCII DEL) is normalized
	// to KeyBackspace by the render/terminal/tcell adapter (spec-1.17
	// D-6a R-10), so we do NOT expose 127 here.
	KeyCtrlC         = int(tcell.KeyCtrlC)         // 67 — quit protocol (spec-1.15 D-5)
	KeyCtrlBackslash = int(tcell.KeyCtrlBackslash) // 92 — hard exit (spec-0.12)
	KeyCtrlA         = int(tcell.KeyCtrlA)         // 65 — cursor home (line start)
	KeyCtrlE         = int(tcell.KeyCtrlE)         // 69 — cursor end (line end)
	KeyDelete        = int(tcell.KeyDelete)        // 271 — forward delete
	KeyUp            = int(tcell.KeyUp)            // 257 — history prev
	KeyDown          = int(tcell.KeyDown)          // 258 — history next
	KeyRight         = int(tcell.KeyRight)         // 259 — cursor right
	KeyLeft          = int(tcell.KeyLeft)          // 260 — cursor left
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
