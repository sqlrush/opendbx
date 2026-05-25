// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package input

import (
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/terminal"
)

// spec-1.16 § 4.3 perf targets (pure functions, no allocations expected
// on the steady-state insert path):
//   - BenchmarkResolveMode_steadyState   < 100 ns/op (KeyRune insert; R4 H-2 rename)
//   - BenchmarkResolveMode_modeSwitch    < 200 ns/op (first-char trigger path)
//   - BenchmarkDeriveMode                <  20 ns/op
//   - BenchmarkValueWithoutPrefix        <  20 ns/op (slice header only)
//
// R4 H-2 fix: rename benches to match spec § 4.3 names so CI perf
// regression baseline can compare by name. BenchmarkPaintInputRow_
// withMode lives in program/bench_test.go (different package).

func BenchmarkDeriveMode(b *testing.B) {
	b.ReportAllocs()
	bufs := []string{"", "/help", "\\select", "hello world"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = DeriveMode(bufs[i&3])
	}
}

// BenchmarkResolveMode_steadyState measures the KeyRune-append hot path.
// R4 NIT-3 doc: each iteration appends one rune; buffer resets at 1024
// bytes to bound the allocation profile (steady-state cost is dominated
// by short-string copy, not long-string realloc).
func BenchmarkResolveMode_steadyState(b *testing.B) {
	b.ReportAllocs()
	buf, cursor := "", 0
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf, cursor = ResolveMode(buf, cursor, terminal.KeyRune, 'a')
		if len(buf) > 1024 {
			buf, cursor = "", 0
		}
	}
}

// BenchmarkResolveMode_modeSwitch measures the first-char path that
// transitions between modes (each iteration starts from empty buffer
// and applies one trigger keypress + DeriveMode lookup at read site).
func BenchmarkResolveMode_modeSwitch(b *testing.B) {
	b.ReportAllocs()
	triggers := []rune{'/', '\\', 'a'}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf, _ := ResolveMode("", 0, terminal.KeyRune, triggers[i%3])
		_ = DeriveMode(buf)
	}
}

func BenchmarkValueWithoutPrefix(b *testing.B) {
	b.ReportAllocs()
	bufs := []string{"/help arg1", "\\select * from t", "hello world", ""}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ValueWithoutPrefix(bufs[i&3])
	}
}
