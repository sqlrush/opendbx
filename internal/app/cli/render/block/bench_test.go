// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Bench raw artifact target: tests/perf/stage1.7-block-bench.txt
// (R2-2 raw pattern; T-8 manual freeze).
//
// spec § 4.3 targets:
//   - render_text:         < 50µs   (wrap + multi-row SetCell)
//   - render_fence:        < 200µs  (fence scan + code-style render)
//   - render_measureonly:  < 5µs    (no cell writes, line count only)

package block

import (
	"strings"
	"testing"
)

func BenchmarkMessage_render_text(b *testing.B) {
	ctx := Context{Cols: 80, Theme: DefaultTheme{}, Wrap: WrapSoft}
	m := Message{Text: strings.Repeat("the quick brown fox jumps\n", 20)}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = m.Render(ctx)
	}
}

func BenchmarkMessage_render_fence(b *testing.B) {
	ctx := Context{Cols: 80, Theme: DefaultTheme{}, Wrap: WrapSoft}
	m := Message{Text: "intro line\n```go\npackage main\n\nfunc main() {\n    println(\"hello\")\n}\n```\nafter fence"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = m.Render(ctx)
	}
}

func BenchmarkMessage_render_measureonly(b *testing.B) {
	ctx := Context{Cols: 80, MeasureOnly: true, Theme: DefaultTheme{}, Wrap: WrapSoft}
	m := Message{Text: strings.Repeat("the quick brown fox jumps\n", 20)}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = m.Render(ctx)
	}
}
