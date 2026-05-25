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
	s.Schedule(func() Msg { called = true; return nil })
	if !called {
		t.Errorf("Schedule did not invoke cmd")
	}
	ch <- Tick{When: time.Now(), Frame: 1}
	if got := <-s.Tick(); got.Frame != 1 {
		t.Errorf("Tick Frame = %d want 1", got.Frame)
	}
}

// TestErrPanicRecovered_Triple verifies the errcode three-piece
// contract per CLAUDE rule 7 and spec-1.4 D-3.
func TestErrPanicRecovered_Triple(t *testing.T) {
	t.Parallel()
	if got := ErrPanicRecovered.Code(); got != "RENDER.SCHEDULER_PANIC_RECOVERED" {
		t.Errorf("Code = %q, want RENDER.SCHEDULER_PANIC_RECOVERED", got)
	}
	if ErrPanicRecovered.Message() == "" {
		t.Errorf("Message must be non-empty")
	}
	if ErrPanicRecovered.Hint() == "" {
		t.Errorf("Hint must be non-empty")
	}
}
