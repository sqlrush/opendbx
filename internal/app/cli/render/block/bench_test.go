// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Bench raw artifact target: tests/perf/stage1.7-block-bench.txt
// (R2-2 raw pattern; T-8 manual freeze).
//
// spec-1.7 § 4.3 targets:
//   - render_text:         < 50µs   (wrap + multi-row SetCell)
//   - render_fence:        < 200µs  (fence scan + code-style render)
//   - render_measureonly:  < 5µs    (no cell writes, line count only)
//
// spec-1.9 § 4.3 targets:
//   - tooluse_render_queued:        < 20µs
//   - tooluse_render_running_bash:  < 50µs   (160-char cmd, no progress)
//   - tooluse_render_with_progress: < 100µs  (5 progress msg)
//   - tooluse_render_measureonly:   < 5µs

package block

import (
	"strings"
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/block/adapter"
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

func BenchmarkToolUse_render_queued(b *testing.B) {
	tu := NewToolUse("idQ", "Bash", map[string]any{"command": "ls"})
	ctx := Context{Cols: 80, Theme: DefaultTheme{}, Wrap: WrapSoft}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = tu.Render(ctx)
	}
}

func BenchmarkToolUse_render_running_bash(b *testing.B) {
	cmd := strings.Repeat("x", 160)
	tu := NewToolUse("idR", "Bash", map[string]any{"command": cmd})
	tu.State = StateRunning
	ctx := Context{Cols: 80, Theme: DefaultTheme{}, Wrap: WrapSoft}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = tu.Render(ctx)
	}
}

func BenchmarkToolUse_render_with_progress(b *testing.B) {
	tu := NewToolUse("idS", "Bash", map[string]any{"command": "long_cmd"})
	tu.State = StateRunning
	tu.ProgressMessages = []adapter.ProgressMessage{
		{ElapsedSeconds: 1, TotalLines: 10},
		{ElapsedSeconds: 2, TotalLines: 20},
		{ElapsedSeconds: 3, TotalLines: 30},
		{ElapsedSeconds: 4, TotalLines: 40},
		{ElapsedSeconds: 5, TotalLines: 50},
	}
	ctx := Context{Cols: 80, Theme: DefaultTheme{}, Wrap: WrapSoft}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = tu.Render(ctx)
	}
}

func BenchmarkToolUse_render_measureonly(b *testing.B) {
	tu := NewToolUse("idT", "Bash", map[string]any{"command": "x"})
	tu.State = StateRunning
	ctx := Context{Cols: 80, MeasureOnly: true, Theme: DefaultTheme{}, Wrap: WrapSoft}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = tu.Render(ctx)
	}
}
