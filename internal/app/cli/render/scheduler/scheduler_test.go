// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package scheduler

import (
	"testing"
	"time"
)

func TestTick_ZeroValue(t *testing.T) {
	t.Parallel()
	tk := Tick{}
	if !tk.When.IsZero() || tk.Frame != 0 {
		t.Errorf("zero Tick: %+v", tk)
	}
}

type fakeScheduler struct{ ch chan Tick }

func (f *fakeScheduler) Schedule(cmd Cmd)  { cmd() }
func (f *fakeScheduler) Tick() <-chan Tick { return f.ch }

func TestScheduler_InterfaceContract(t *testing.T) {
	t.Parallel()
	ch := make(chan Tick, 1)
	var s Scheduler = &fakeScheduler{ch: ch}
	called := false
	s.Schedule(func() { called = true })
	if !called {
		t.Errorf("Schedule did not invoke cmd")
	}
	ch <- Tick{When: time.Now(), Frame: 1}
	if got := <-s.Tick(); got.Frame != 1 {
		t.Errorf("Tick Frame = %d want 1", got.Frame)
	}
}
