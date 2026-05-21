// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package scrollback

import (
	"sync"
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/layout"
)

// #15 TestConcurrent_PushFromWorkers (CLAUDE 规则 9 race + R-8).
// 8 goroutines × 100 Push concurrent with 1 main-loop Render.
//
// Pattern mirrors spec-1.4 scheduler worker pool: workers call Push via
// Cmd from many goroutines; the scheduler's Render fires from the main
// loop. The internal Lock matrix (R2 D5) must serialize without deadlock,
// data race, or panic.
func TestConcurrent_PushFromWorkers(t *testing.T) {
	sb := NewVirtualScrollback(WithMaxNodes(2000))
	// Prime with a Render so lastCols/Rows are set; subsequent Pushes
	// can use the real measure path.
	sb.Render(mustGrid(t, 80, 24), layout.Box{Width: 80, Height: 24})

	const workers = 8
	const pushPerWorker = 100

	stopRender := make(chan struct{})
	renderDone := make(chan struct{})
	go func() {
		defer close(renderDone)
		next := mustGrid(t, 80, 24)
		for {
			select {
			case <-stopRender:
				return
			default:
				sb.Render(next, layout.Box{Width: 80, Height: 24})
			}
		}
	}()

	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < pushPerWorker; i++ {
				sb.Push(fakeBlock{height: 1, tag: rune('A' + workerID%26)})
			}
		}(w)
	}
	wg.Wait()
	close(stopRender)
	<-renderDone

	if got := sb.Len(); got != workers*pushPerWorker {
		t.Fatalf("Len()=%d, want %d (8×100)", got, workers*pushPerWorker)
	}
}

// TestConcurrent_ReadersDuringWriters (R2 D5 RLock readers).
// Range/Len/IsSticky/ScrollY readers concurrent with Push writers; race
// detector clean; readers never observe torn state.
func TestConcurrent_ReadersDuringWriters(t *testing.T) {
	sb := NewVirtualScrollback()
	sb.Render(mustGrid(t, 80, 24), layout.Box{Width: 80, Height: 24})

	const writers = 4
	const writes = 50
	const readers = 4
	const reads = 200

	var wg sync.WaitGroup
	wg.Add(writers + readers)
	for i := 0; i < writers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < writes; j++ {
				sb.Push(fakeBlock{height: 1, tag: 'X'})
			}
		}()
	}
	for i := 0; i < readers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < reads; j++ {
				_ = sb.Len()
				_ = sb.IsSticky()
				_ = sb.ScrollY()
				_ = sb.Range(0, 5)
			}
		}()
	}
	wg.Wait()

	if got := sb.Len(); got != writers*writes {
		t.Fatalf("Len()=%d, want %d", got, writers*writes)
	}
}
