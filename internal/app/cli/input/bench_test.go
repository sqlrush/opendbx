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
//   - BenchmarkDeriveMode               <  20 ns/op
//   - BenchmarkResolveMode_Insert       < 100 ns/op (string concat costs)
//   - BenchmarkValueWithoutPrefix       <  20 ns/op (slice header only)

func BenchmarkDeriveMode(b *testing.B) {
	b.ReportAllocs()
	bufs := []string{"", "/help", "\\select", "hello world"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = DeriveMode(bufs[i&3])
	}
}

func BenchmarkResolveMode_Insert(b *testing.B) {
	b.ReportAllocs()
	buf, cursor := "", 0
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf, cursor = ResolveMode(buf, cursor, terminal.KeyRune, 'a')
		if len(buf) > 1024 {
			// reset to avoid unbounded growth dominating the bench
			buf, cursor = "", 0
		}
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
