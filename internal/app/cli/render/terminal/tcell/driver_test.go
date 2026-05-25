// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package tcell

import (
	"context"
	"sync"
	"testing"
	"time"

	tcellv2 "github.com/gdamore/tcell/v2"

	"github.com/sqlrush/opendbx/internal/app/cli/render/style"
	"github.com/sqlrush/opendbx/internal/app/cli/render/terminal"
)

// TestFromTcellEventKey_MappingTable verifies the 13-case translation
// table from tcell.Key codes to terminal.Key* codes, including the
// KeyBackspace2 (127 = DEL) → KeyBackspace (8) normalization that is
// the key R-10 invariant for spec-1.17 cursor edit.
func TestFromTcellEventKey_MappingTable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		key      tcellv2.Key
		rune     rune
		mod      tcellv2.ModMask
		wantCode int
		wantRune rune
		wantMod  uint8
	}{
		{"Backspace (BS 8)", tcellv2.KeyBackspace, 0, 0, terminal.KeyBackspace, 0, 0},
		{"Backspace2 (DEL 127) normalize", tcellv2.KeyBackspace2, 0, 0, terminal.KeyBackspace, 0, 0},
		{"Delete (271)", tcellv2.KeyDelete, 0, 0, terminal.KeyDelete, 0, 0},
		{"Left (260)", tcellv2.KeyLeft, 0, 0, terminal.KeyLeft, 0, 0},
		{"Right (259)", tcellv2.KeyRight, 0, 0, terminal.KeyRight, 0, 0},
		{"Up (257)", tcellv2.KeyUp, 0, 0, terminal.KeyUp, 0, 0},
		{"Down (258)", tcellv2.KeyDown, 0, 0, terminal.KeyDown, 0, 0},
		// tcell.NewEventKey populates Rune from the Ctrl-letter (a/e/c) for the
		// KeyCtrl* family per its normalize loop. The adapter passes Rune through
		// — keybindings.Resolve matches on Code so Rune is informational.
		{"CtrlA (65)", tcellv2.KeyCtrlA, 0, tcellv2.ModCtrl, terminal.KeyCtrlA, 'a', terminal.ModCtrl},
		{"CtrlE (69)", tcellv2.KeyCtrlE, 0, tcellv2.ModCtrl, terminal.KeyCtrlE, 'e', terminal.ModCtrl},
		{"Enter (13)", tcellv2.KeyEnter, 0, 0, terminal.KeyEnter, 0, 0},
		{"Escape (27)", tcellv2.KeyEscape, 0, 0, terminal.KeyEscape, 0, 0},
		{"CtrlC (67)", tcellv2.KeyCtrlC, 0, tcellv2.ModCtrl, terminal.KeyCtrlC, 'c', terminal.ModCtrl},
		{"CtrlBackslash (28)", tcellv2.KeyCtrlBackslash, 0, tcellv2.ModCtrl, terminal.KeyCtrlBackslash, 0, terminal.ModCtrl},
		{"Rune 'a'", tcellv2.KeyRune, 'a', 0, terminal.KeyRune, 'a', 0},
		{"Rune '中' (CJK)", tcellv2.KeyRune, '中', 0, terminal.KeyRune, '中', 0},
		{"Shift+Left", tcellv2.KeyLeft, 0, tcellv2.ModShift, terminal.KeyLeft, 0, terminal.ModShift},
		{"Ctrl+Alt+Rune 'x'", tcellv2.KeyRune, 'x', tcellv2.ModCtrl | tcellv2.ModAlt, terminal.KeyRune, 'x', terminal.ModCtrl | terminal.ModAlt},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev := tcellv2.NewEventKey(tc.key, tc.rune, tc.mod)
			got := fromTcellEventKey(ev)
			if got.Code != tc.wantCode {
				t.Errorf("Code = %d; want %d", got.Code, tc.wantCode)
			}
			if got.Rune != tc.wantRune {
				t.Errorf("Rune = %q; want %q", got.Rune, tc.wantRune)
			}
			if got.ShiftCtrlAlt != tc.wantMod {
				t.Errorf("Mod = %d; want %d", got.ShiftCtrlAlt, tc.wantMod)
			}
		})
	}
}

// TestFromTcellMod_AllCombinations verifies ModShift / ModCtrl / ModAlt
// bit conversion and ensures ModMeta is silently dropped (opendbx does
// not surface Meta in keybindings).
func TestFromTcellMod_AllCombinations(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   tcellv2.ModMask
		want uint8
	}{
		{"none", 0, 0},
		{"shift only", tcellv2.ModShift, terminal.ModShift},
		{"ctrl only", tcellv2.ModCtrl, terminal.ModCtrl},
		{"alt only", tcellv2.ModAlt, terminal.ModAlt},
		{"ctrl+shift", tcellv2.ModShift | tcellv2.ModCtrl, terminal.ModShift | terminal.ModCtrl},
		{"ctrl+alt+shift", tcellv2.ModShift | tcellv2.ModCtrl | tcellv2.ModAlt, terminal.ModShift | terminal.ModCtrl | terminal.ModAlt},
		{"meta dropped", tcellv2.ModMeta, 0},
		{"ctrl+meta → only ctrl", tcellv2.ModCtrl | tcellv2.ModMeta, terminal.ModCtrl},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := fromTcellMod(tc.in); got != tc.want {
				t.Errorf("fromTcellMod(%v) = %d; want %d", tc.in, got, tc.want)
			}
		})
	}
}

// TestPollEvent_HappyPath drives a SimulationScreen through Init → Inject
// a KeyCtrlC press → PollEvent should return terminal.EventKey with
// Code=KeyCtrlC, Mod=ModCtrl.
func TestPollEvent_HappyPath(t *testing.T) {
	t.Parallel()
	sim := tcellv2.NewSimulationScreen("UTF-8")
	d := NewDriver(sim)
	if err := d.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer d.Fini()

	go func() {
		// Give PollEvent time to be waiting.
		time.Sleep(10 * time.Millisecond)
		sim.InjectKey(tcellv2.KeyCtrlC, 0, tcellv2.ModCtrl)
	}()

	ev, err := d.PollEvent(context.Background())
	if err != nil {
		t.Fatalf("PollEvent: %v", err)
	}
	ek, ok := ev.(terminal.EventKey)
	if !ok {
		t.Fatalf("PollEvent returned %T; want terminal.EventKey", ev)
	}
	if ek.Code != terminal.KeyCtrlC {
		t.Errorf("Code = %d; want KeyCtrlC=%d", ek.Code, terminal.KeyCtrlC)
	}
	if ek.ShiftCtrlAlt != terminal.ModCtrl {
		t.Errorf("Mod = %d; want ModCtrl=%d", ek.ShiftCtrlAlt, terminal.ModCtrl)
	}
}

// TestPollEvent_CtxCancelled verifies that a pre-cancelled context
// short-circuits PollEvent without blocking on the tcell queue.
func TestPollEvent_CtxCancelled(t *testing.T) {
	t.Parallel()
	sim := tcellv2.NewSimulationScreen("UTF-8")
	d := NewDriver(sim)
	if err := d.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer d.Fini()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := d.PollEvent(ctx)
	if err != context.Canceled {
		t.Errorf("PollEvent returned err=%v; want context.Canceled", err)
	}
}

// TestPollEvent_Resize asserts that a tcell EventResize translates to
// terminal.EventResize and updates the cached cols/rows.
//
// tcell's SimulationScreen.SetSize updates internal state silently —
// it does NOT enqueue an EventResize. To exercise the resize path the
// test posts a synthetic EventResize directly.
func TestPollEvent_Resize(t *testing.T) {
	t.Parallel()
	sim := tcellv2.NewSimulationScreen("UTF-8")
	d := NewDriver(sim)
	if err := d.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer d.Fini()

	sim.SetSize(100, 30)
	if err := sim.PostEvent(tcellv2.NewEventResize(100, 30)); err != nil {
		t.Fatalf("PostEvent(Resize): %v", err)
	}

	ev, err := d.PollEvent(context.Background())
	if err != nil {
		t.Fatalf("PollEvent: %v", err)
	}
	er, ok := ev.(terminal.EventResize)
	if !ok {
		t.Fatalf("PollEvent returned %T; want terminal.EventResize", ev)
	}
	if er.Cols != 100 || er.Rows != 30 {
		t.Errorf("Resize Cols/Rows = %d/%d; want 100/30", er.Cols, er.Rows)
	}
	if c, r := d.Size(); c != 100 || r != 30 {
		t.Errorf("cached Size after resize = %d/%d; want 100/30", c, r)
	}
}

// TestPostEvent_InterruptRoundtrip writes an EventInterrupt with a
// custom payload and reads it back via PollEvent.
func TestPostEvent_InterruptRoundtrip(t *testing.T) {
	t.Parallel()
	sim := tcellv2.NewSimulationScreen("UTF-8")
	d := NewDriver(sim)
	if err := d.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer d.Fini()

	payload := "spec-1.17-test-payload"
	if err := d.PostEvent(terminal.EventInterrupt{Data: payload}); err != nil {
		t.Fatalf("PostEvent: %v", err)
	}

	ev, err := d.PollEvent(context.Background())
	if err != nil {
		t.Fatalf("PollEvent: %v", err)
	}
	ei, ok := ev.(terminal.EventInterrupt)
	if !ok {
		t.Fatalf("PollEvent returned %T; want terminal.EventInterrupt", ev)
	}
	if ei.Data != payload {
		t.Errorf("Data = %v; want %v", ei.Data, payload)
	}
}

// TestPostEvent_UnsupportedReturnsNil verifies that posting a
// terminal.Event type the adapter does not round-trip (e.g.
// EventKey/EventResize) silently no-ops instead of erroring.
func TestPostEvent_UnsupportedReturnsNil(t *testing.T) {
	t.Parallel()
	sim := tcellv2.NewSimulationScreen("UTF-8")
	d := NewDriver(sim)
	if err := d.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer d.Fini()

	if err := d.PostEvent(terminal.EventKey{Code: terminal.KeyEnter}); err != nil {
		t.Errorf("PostEvent(EventKey) returned %v; want nil (silent no-op)", err)
	}
}

// TestFiniIdempotent calls Fini multiple times and asserts no panic
// (sync.Once protects screen.Fini).
func TestFiniIdempotent(t *testing.T) {
	t.Parallel()
	sim := tcellv2.NewSimulationScreen("UTF-8")
	d := NewDriver(sim)
	if err := d.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	d.Fini()
	d.Fini() // second call must be a no-op
	d.Fini() // third too
}

// TestInitIdempotent calls Init multiple times and asserts no panic
// or duplicate-Init error.
func TestInitIdempotent(t *testing.T) {
	t.Parallel()
	sim := tcellv2.NewSimulationScreen("UTF-8")
	d := NewDriver(sim)
	if err := d.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := d.Init(); err != nil {
		t.Errorf("second Init returned %v; want nil (sync.Once protected)", err)
	}
	defer d.Fini()
}

// TestNewDriverNilPanics asserts the constructor refuses a nil screen.
func TestNewDriverNilPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("NewDriver(nil) did not panic")
		}
	}()
	_ = NewDriver(nil)
}

// TestSetCell_StyleConversion exercises toTcellStyle indirectly by
// writing a styled cell and reading the back-buffer via tcell's
// GetContent helper (SimulationScreen exposes the cell content).
func TestSetCell_StyleConversion(t *testing.T) {
	t.Parallel()
	sim := tcellv2.NewSimulationScreen("UTF-8")
	d := NewDriver(sim)
	if err := d.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer d.Fini()

	sim.SetSize(20, 5)
	// Synthetic Resize event so cached cols/rows match the SetSize.
	d.cols, d.rows = 20, 5

	st := style.Style{
		FG:        style.RGB(255, 128, 0),
		Bold:      true,
		Underline: true,
	}
	d.SetCell(2, 1, 'X', st)
	d.Show()

	got, _, _, _ := sim.GetContent(2, 1)
	if got != 'X' {
		t.Errorf("cell rune = %q; want 'X'", got)
	}
}

// TestConcurrentFini ensures concurrent Fini calls don't double-close
// the underlying screen (sync.Once contract).
func TestConcurrentFini(t *testing.T) {
	t.Parallel()
	sim := tcellv2.NewSimulationScreen("UTF-8")
	d := NewDriver(sim)
	if err := d.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d.Fini()
		}()
	}
	wg.Wait()
}
