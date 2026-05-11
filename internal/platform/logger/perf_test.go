// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Performance baselines for the logger package (spec § 9 perf layer).
//
// Run via:
//
//	go test -bench=. -benchmem ./internal/platform/logger/
//
// Baseline targets (spec § 9, codex LOW-2 + claude perf alignment):
//
//   - BenchmarkBufferedWriterImmediate: ≥ 50k events / s
//   - BenchmarkMarshalSidecarEvent:     < 50 µs / event
//   - BenchmarkFlushLatencyP99:         < 1.5 × flushIntervalMs
//
// CI integration arrives with spec-0.11 perf harness; for now these are
// regression baselines a developer can run locally and compare against
// spec § 9 numbers.

package logger

import (
	"sync"
	"testing"
	"time"
)

// BenchmarkBufferedWriterImmediate measures throughput of the BufferedWriter
// in immediateMode (the hot path under IsDebugMode=true). Target: ≥ 50k ops/s
// — on a modern laptop we expect ~500k ops/s+ since the writeFn here is a
// no-op.
func BenchmarkBufferedWriterImmediate(b *testing.B) {
	cfg := defaultBufferedWriterConfig(func(string) error { return nil })
	cfg.immediateMode = true
	bw := newBufferedWriter(cfg)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bw.Write("payload")
	}
	b.StopTimer()
	_ = bw.Dispose()
	reportOpsPerSec(b)
}

// BenchmarkBufferedWriterBuffered measures throughput when buffering is
// active (immediateMode=false, drained via timer / Dispose).
func BenchmarkBufferedWriterBuffered(b *testing.B) {
	cfg := defaultBufferedWriterConfig(func(string) error { return nil })
	cfg.maxBufferSize = 1024
	bw := newBufferedWriter(cfg)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bw.Write("payload")
	}
	b.StopTimer()
	_ = bw.Dispose()
	reportOpsPerSec(b)
}

// BenchmarkMarshalSidecarEvent measures the JSON marshalling cost for one
// sidecar record. Target: < 50 µs / event. We exercise a realistic shape:
// 6 attrs, populated trace/span IDs.
func BenchmarkMarshalSidecarEvent(b *testing.B) {
	now := time.Now()
	merged := []Attr{
		{Key: "module", Value: "llm"},
		{Key: "duration_ms", Value: 42},
		{Key: "tokens_used", Value: 12345},
		{Key: "model", Value: "claude-opus-4-7"},
		{Key: "tier", Value: "tier-1"},
		{Key: "stream", Value: true},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := marshalSidecarEvent(
			now, LevelInfo, "llm", "stream chunk received",
			"sess-abc",
			merged,
			"019e14ce-3cf4-7000-afc2-97bffd644ff8",
			"019e14ce-3cf4-7001-9414-2dfba895c18d",
		)
		if err != nil {
			b.Fatalf("marshal err = %v", err)
		}
	}
}

// BenchmarkFormatEvent measures the CC text formatter throughput. This is
// the first hot-path step inside log() for every emitted event.
func BenchmarkFormatEvent(b *testing.B) {
	now := time.Now()
	for _, mode := range []struct {
		name      string
		formatted bool
		msg       string
	}{
		{"single-line/pre-tui", false, "api: connected to db"},
		{"single-line/post-tui", true, "api: connected to db"},
		{"multi-line/post-tui-jsonify", true, "line1\nline2\nline3"},
	} {
		mode := mode
		b.Run(mode.name, func(b *testing.B) {
			SetHasFormattedOutput(mode.formatted)
			defer SetHasFormattedOutput(false)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = formatEvent(now, LevelInfo, mode.msg)
			}
		})
	}
}

// BenchmarkRedactString measures secret-pattern scan cost on representative
// payloads (no-secret common case + worst-case all patterns).
func BenchmarkRedactString(b *testing.B) {
	for _, c := range []struct {
		name string
		in   string
	}{
		{"clean", "api: connected to remote service for query"},
		{"one_secret", "POST /login password=hunter2 content"},
		{"many_secrets", `Authorization: Bearer abcdef ghi; password=foo; token=bar; sk-abc123def456 conn postgres://u:p@h/x`},
	} {
		c := c
		b.Run(c.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = redactString(c.in)
			}
		})
	}
}

// BenchmarkUUID7 measures cost of v7 generation under the monotonic lock.
// Target: < 1 µs / op on modern hardware.
func BenchmarkUUID7(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = uuid7()
	}
}

// BenchmarkEndToEndLog measures the full log() path with main writer +
// sidecar both wired (the realistic logger.L().Info(...) cost). Useful to
// track regressions when D-9 (redaction) or D-5 (sidecar) evolves.
func BenchmarkEndToEndLog(b *testing.B) {
	resetForBench(b)
	cfg := defaultBufferedWriterConfig(func(string) error { return nil })
	cfg.immediateMode = true
	mainW := newBufferedWriter(cfg)
	sideW := newBufferedWriter(cfg)
	impl := &loggerImpl{
		mu:             &sync.Mutex{},
		minLevel:       LevelDebug,
		sessionID:      "bench",
		debugToStderr:  true,
		sidecarEnabled: true,
		mainWriter:     mainW,
		sidecarWriter:  sideW,
	}
	current.Store(impl)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		impl.log(LevelInfo, "api: tick", []Attr{{Key: "event", Value: "tick"}})
	}
}

// reportOpsPerSec annotates the benchmark with an ops/sec metric so the
// reader can directly compare against the spec § 9 baseline targets.
func reportOpsPerSec(b *testing.B) {
	if b.N == 0 || b.Elapsed() == 0 {
		return
	}
	opsPerSec := float64(b.N) / b.Elapsed().Seconds()
	b.ReportMetric(opsPerSec, "ops/s")
}

// resetForBench is a benchmark-only test reset (cannot reuse resetForTesting
// because that takes *testing.T not *testing.B).
func resetForBench(b *testing.B) {
	b.Helper()
	current.Store(nil)
	runtimeDebugEnabled.Store(false)
	hasFormattedOutput.Store(false)
}
