// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package streaming

import (
	"context"
	"sync"
	"testing"
)

// #18 TestConcurrent_AppendFromGoroutines (CLAUDE 规则 9 race + R-8).
// 8 producer goroutines × 50 AppendChunk concurrent with main-loop Drain
// + Close. Race detector must report 0 issues.
func TestConcurrent_AppendFromGoroutines(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := NewTokenStream(ctx, WithChanCapacity(64))

	const producers = 8
	const writes = 50

	var wg sync.WaitGroup
	wg.Add(producers)
	for i := 0; i < producers; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < writes; j++ {
				_ = s.AppendChunk(Chunk{Token: "x\n"})
			}
		}(i)
	}

	// Concurrent drainer mimicking scheduler main loop.
	drainDone := make(chan struct{})
	go func() {
		defer close(drainDone)
		for {
			select {
			case <-ctx.Done():
				return
			default:
				_ = s.Drain()
			}
		}
	}()

	wg.Wait()
	cancel()
	<-drainDone
	_ = s.Close()
	_ = s.Drain()
}
