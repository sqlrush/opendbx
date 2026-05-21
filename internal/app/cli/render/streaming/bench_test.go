// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Bench raw artifact target: tests/perf/stage1.6-streaming-bench.txt
// (R2-2 raw pattern, no auto gate).
//
// T-8 wrap-up manually freezes baseline:
//
//	go test -bench=BenchmarkStream -benchmem -run=^$ \
//	    ./internal/app/cli/render/streaming/... -benchtime=2s \
//	    | tee tests/perf/stage1.6-streaming-bench.txt
//
// spec § 4.3 targets:
//   - Append_per_token:        < 5µs   (Lock + ctx pre-select + chan send)
//   - Drain_per_frame:         < 100µs (16 chunks drain + 2 line emits)
//   - FullSession_10k_token:   < 100ms (10k AppendChunk + Drain cycles + Close)

package streaming

import (
	"context"
	"testing"
)

// BenchmarkStream_Append_per_token measures single-chunk AppendChunk
// latency with a consumer goroutine draining in parallel (so chan is
// not the bottleneck).
func BenchmarkStream_Append_per_token(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := NewTokenStream(ctx, WithChanCapacity(1024))

	// Consumer goroutine.
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				_ = s.Drain()
			}
		}
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.AppendChunk(Chunk{Token: "x"})
	}
}

// BenchmarkStream_Drain_per_frame measures one Drain call processing
// ~16 chunks (typical frame at 60fps with ~200 tok/s LLM).
func BenchmarkStream_Drain_per_frame(b *testing.B) {
	const chunksPerFrame = 16
	for n := 0; n < b.N; n++ {
		b.StopTimer()
		s := NewTokenStream(context.Background(), WithChanCapacity(chunksPerFrame*2))
		for i := 0; i < chunksPerFrame; i++ {
			tok := "x"
			if i%8 == 7 {
				tok = "x\n" // 2 complete lines per frame
			}
			_ = s.AppendChunk(Chunk{Token: tok})
		}
		b.StartTimer()
		_ = s.Drain()
	}
}

// BenchmarkStream_FullSession_10k_token simulates a full LLM response:
// 10k token-chunks + periodic Drain (every 16 chunks) + final FinishLength.
func BenchmarkStream_FullSession_10k_token(b *testing.B) {
	for n := 0; n < b.N; n++ {
		b.StopTimer()
		ctx, cancel := context.WithCancel(context.Background())
		s := NewTokenStream(ctx, WithChanCapacity(64))
		b.StartTimer()

		for i := 0; i < 10000; i++ {
			tok := "tok "
			if i%32 == 31 {
				tok = "line\n"
			}
			_ = s.AppendChunk(Chunk{Token: tok})
			if i%16 == 15 {
				_ = s.Drain()
			}
		}
		_ = s.AppendChunk(Chunk{FinishReason: FinishLength})
		_ = s.Close()
		_ = s.Drain()

		b.StopTimer()
		cancel()
		b.StartTimer()
	}
}
