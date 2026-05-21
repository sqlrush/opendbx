// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package scheduler

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestQueue_StrictPriority — high → normal → idle FIFO drain.
func TestQueue_StrictPriority(t *testing.T) {
	t.Parallel()
	q := newQueue()
	q.push(jobItem{Cmd: func() {}, CmdID: 1, Priority: PriorityIdle})
	q.push(jobItem{Cmd: func() {}, CmdID: 2, Priority: PriorityNormal})
	q.push(jobItem{Cmd: func() {}, CmdID: 3, Priority: PriorityHigh})
	q.push(jobItem{Cmd: func() {}, CmdID: 4, Priority: PriorityNormal})
	q.push(jobItem{Cmd: func() {}, CmdID: 5, Priority: PriorityHigh})

	want := []uint64{3, 5, 2, 4, 1} // high(3,5) → normal(2,4) → idle(1)
	for i, w := range want {
		j, ok := q.pop()
		if !ok {
			t.Fatalf("pop #%d: expected jobItem, got empty", i)
		}
		if j.CmdID != w {
			t.Errorf("pop #%d: CmdID = %d, want %d (strict-priority order)", i, j.CmdID, w)
		}
	}
	if _, ok := q.pop(); ok {
		t.Errorf("pop on drained queue should return false")
	}
}

func TestQueue_EmptyPop(t *testing.T) {
	t.Parallel()
	q := newQueue()
	j, ok := q.pop()
	if ok {
		t.Errorf("empty pop ok = true; want false (got %+v)", j)
	}
}

// TestQueue_ConcurrentPush — 16 goroutines × 100 push to mixed lanes;
// race detector clean; total count = 1600.
func TestQueue_ConcurrentPush(t *testing.T) {
	t.Parallel()
	q := newQueue()
	const goroutines = 16
	const iters = 100
	var wg sync.WaitGroup
	var counter atomic.Uint64
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(seed int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				p := Priority((seed + i) % 3)
				q.push(jobItem{
					Cmd:      func() {},
					CmdID:    counter.Add(1),
					Priority: p,
				})
			}
		}(g)
	}
	wg.Wait()
	if got := q.len(); got != goroutines*iters {
		t.Errorf("total queued = %d, want %d", got, goroutines*iters)
	}
}

// TestQueue_UnknownPriorityCollapsesToNormal — defensive: any
// non-{high,idle} value goes to the normal lane.
func TestQueue_UnknownPriorityCollapsesToNormal(t *testing.T) {
	t.Parallel()
	q := newQueue()
	q.push(jobItem{Cmd: func() {}, CmdID: 1, Priority: Priority(99)})
	j, ok := q.pop()
	if !ok || j.CmdID != 1 {
		t.Errorf("pop unknown priority: %+v ok=%v", j, ok)
	}
}

// TestQueue_FIFOWithinLane — ensures same-lane ordering is FIFO.
func TestQueue_FIFOWithinLane(t *testing.T) {
	t.Parallel()
	q := newQueue()
	for i := uint64(1); i <= 5; i++ {
		q.push(jobItem{Cmd: func() {}, CmdID: i, Priority: PriorityNormal})
	}
	for i := uint64(1); i <= 5; i++ {
		j, _ := q.pop()
		if j.CmdID != i {
			t.Errorf("FIFO violated: got %d want %d", j.CmdID, i)
		}
	}
}

// TestQueue_PushDuringPopRace — interleave push from background
// goroutines while popping from foreground. race detector must stay
// silent and final counts must reconcile.
func TestQueue_PushDuringPopRace(t *testing.T) {
	t.Parallel()
	q := newQueue()
	const pushers = 4
	const perPusher = 50
	var wg sync.WaitGroup
	wg.Add(pushers)
	for g := 0; g < pushers; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < perPusher; i++ {
				q.push(jobItem{Cmd: func() {}, Priority: PriorityNormal})
				time.Sleep(time.Microsecond)
			}
		}()
	}
	popped := 0
	stop := time.After(50 * time.Millisecond)
	for {
		select {
		case <-stop:
			wg.Wait()
			// drain remainder
			for {
				if _, ok := q.pop(); !ok {
					if popped+q.len() != pushers*perPusher {
						t.Errorf("popped %d + remaining %d != total %d", popped, q.len(), pushers*perPusher)
					}
					return
				}
				popped++
			}
		default:
			if _, ok := q.pop(); ok {
				popped++
			}
		}
	}
}
