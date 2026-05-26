// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package tcell

import (
	tcellv2 "github.com/gdamore/tcell/v2"

	"github.com/sqlrush/opendbx/internal/app/cli/render/terminal"
)

// NewSimulationDriverForTest constructs a Driver backed by a tcell
// SimulationScreen sized cols×rows. spec-1.17 D-7 integration seam: it
// lets sibling test packages (tests/integration/uitest/demoapp) drive
// the full production stack WITHOUT importing tcell directly, keeping
// the IMP-9 tcell-isolation boundary at the 4 whitelisted packages.
//
// The screen is returned un-init'd (Driver.Init owns Init, invoked by
// scheduler.Run) — matching the production NewDriver contract.
//
// Test code only.
func NewSimulationDriverForTest(cols, rows int) *Driver {
	sim := tcellv2.NewSimulationScreen("UTF-8")
	sim.SetSize(cols, rows)
	return NewDriver(sim)
}

// InjectKeyForTest injects a key event into the Driver's underlying
// SimulationScreen using a terminal.Key* code (and rune for KeyRune).
// Panics if the Driver does not wrap a SimulationScreen.
//
// terminal.Key* codes are int(tcell.Key*) aliases (spec-1.17 R2 D-1),
// so the code casts directly to tcell.Key. Modifier defaults to none;
// dispatch (keybindings.Resolve / program.preDispatchSystem) keys on
// Code only, so the modifier is not load-bearing for tests.
//
// Test code only.
func (d *Driver) InjectKeyForTest(code int, r rune) {
	sim, ok := d.screen.(tcellv2.SimulationScreen)
	if !ok {
		panic("InjectKeyForTest: driver does not wrap a SimulationScreen")
	}
	mod := tcellv2.ModNone
	// Ctrl-family codes carry ModCtrl so tcell.NewEventKey produces the
	// faithful control event (matches a real terminal Ctrl keypress).
	switch code {
	case terminal.KeyCtrlA, terminal.KeyCtrlC, terminal.KeyCtrlE, terminal.KeyCtrlBackslash:
		mod = tcellv2.ModCtrl
	}
	// spec-1.17 D-6a: terminal.Key codes are bounded tcell iota values
	// (max KeyDelete=271), well within int16 — the conversion cannot overflow.
	//nolint:gosec // spec-1.17 D-6a: terminal.Key codes ≤ 271 fit int16 (tcell.Key); bounded by construction, G115 cannot prove it.
	sim.InjectKey(tcellv2.Key(code), r, mod)
}

// SimReadyForTest reports whether the underlying SimulationScreen has
// been initialized (non-zero width). spec-1.17 D-7 seam; callers should
// prefer program.WaitStartedForTest for happens-before correctness.
func (d *Driver) SimReadyForTest() bool {
	sim, ok := d.screen.(tcellv2.SimulationScreen)
	if !ok {
		return false
	}
	_, w, _ := sim.GetContents()
	return w > 0
}
