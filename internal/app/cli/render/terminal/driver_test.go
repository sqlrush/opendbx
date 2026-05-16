// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package terminal

import (
	"context"
	"errors"
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/style"
)

// --- Event marker contract ------------------------------------------

func TestEvent_TypeTags(t *testing.T) {
	t.Parallel()
	// All three concrete types satisfy Event marker.
	var e Event
	e = EventKey{Code: KeyCtrlC}
	_ = e.(Event)
	e = EventResize{Cols: 80, Rows: 24}
	_ = e.(Event)
	e = EventInterrupt{Data: "ctx-done"}
	_ = e.(Event)
}

func TestEventKey_Modifiers(t *testing.T) {
	t.Parallel()
	ek := EventKey{Code: KeyRune, Rune: 'a', ShiftCtrlAlt: ModCtrl | ModShift}
	if ek.ShiftCtrlAlt&ModCtrl == 0 {
		t.Errorf("ShiftCtrlAlt missing ModCtrl bit")
	}
	if ek.ShiftCtrlAlt&ModShift == 0 {
		t.Errorf("ShiftCtrlAlt missing ModShift bit")
	}
	if ek.ShiftCtrlAlt&ModAlt != 0 {
		t.Errorf("ShiftCtrlAlt should not have ModAlt bit")
	}
}

// --- Driver interface compile-time check (10 methods) ---------------

// fakeDriver implements Driver to verify the interface surface compiles.
type fakeDriver struct{}

func (fakeDriver) Init() error                                  { return nil }
func (fakeDriver) Fini()                                        {}
func (fakeDriver) Show()                                        {}
func (fakeDriver) Sync()                                        {}
func (fakeDriver) Clear()                                       {}
func (fakeDriver) Size() (int, int)                             { return 80, 24 }
func (fakeDriver) SetCell(_, _ int, _ rune, _ style.Style)      {}
func (fakeDriver) PollEvent(ctx context.Context) (Event, error) { return nil, ctx.Err() }
func (fakeDriver) PostEvent(_ Event) error                      { return nil }
func (fakeDriver) Resize(_, _ int)                              {}

func TestDriver_InterfaceSurface(t *testing.T) {
	t.Parallel()
	var d Driver = fakeDriver{}
	if err := d.Init(); err != nil {
		t.Errorf("Init: %v", err)
	}
	if c, r := d.Size(); c != 80 || r != 24 {
		t.Errorf("Size = %d,%d want 80,24", c, r)
	}
	d.SetCell(0, 0, 'a', style.Style{})
	d.Show()
	d.Sync()
	d.Clear()
	d.Resize(120, 40)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := d.PollEvent(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("PollEvent cancelled ctx: want Canceled, got %v", err)
	}
	if err := d.PostEvent(EventInterrupt{}); err != nil {
		t.Errorf("PostEvent: %v", err)
	}
	d.Fini()
}

func TestKeyConstants_NoCollision(t *testing.T) {
	t.Parallel()
	// Spot-check key codes don't collide on numeric value.
	if KeyCtrlC == KeyEnter || KeyCtrlC == KeyEscape || KeyEnter == KeyEscape {
		t.Errorf("key constants collide")
	}
}

func TestEventKey_HasModAndString(t *testing.T) {
	t.Parallel()
	ek := EventKey{Code: KeyRune, Rune: 'a', ShiftCtrlAlt: ModCtrl | ModShift}
	if !ek.HasMod(ModCtrl) || !ek.HasMod(ModShift) {
		t.Errorf("HasMod missing flags")
	}
	if ek.HasMod(ModAlt) {
		t.Errorf("HasMod(ModAlt) should be false")
	}
	if got := ek.ModifierString(); got != "Ctrl+Shift" {
		t.Errorf("ModifierString = %q want Ctrl+Shift", got)
	}
	if (EventKey{}).ModifierString() != "" {
		t.Errorf("zero EventKey.ModifierString() should be empty")
	}
	if got := (EventKey{ShiftCtrlAlt: ModAlt}).ModifierString(); got != "Alt" {
		t.Errorf("Alt-only ModifierString = %q want Alt", got)
	}
}
