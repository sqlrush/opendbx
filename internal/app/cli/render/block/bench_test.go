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
	"fmt"
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

func BenchmarkToolResult_render_success(b *testing.B) {
	tr := NewToolResult("idR1", "Bash", "stdout line\nstdout line 2", false)
	ctx := Context{Cols: 80, Theme: DefaultTheme{}, Wrap: WrapSoft}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = tr.Render(ctx)
	}
}

func BenchmarkToolResult_render_error(b *testing.B) {
	tr := NewToolResult("idR2", "Bash", "command not found: xyz", true)
	ctx := Context{Cols: 80, Theme: DefaultTheme{}, Wrap: WrapSoft}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = tr.Render(ctx)
	}
}

func BenchmarkToolResult_render_measureonly(b *testing.B) {
	tr := NewToolResult("idR3", "Bash", "x", false)
	ctx := Context{Cols: 80, MeasureOnly: true, Theme: DefaultTheme{}, Wrap: WrapSoft}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = tr.Render(ctx)
	}
}

// spec-1.10 § 4.3 targets (R5 M-2 absorb: comment realigned to spec
// § 4.3; collapsed_large fixture realigned to "20 reads + 10 searches"
// per spec line 220):
//   - compact_render_collapsed_small:   < 30µs   (1 read group)
//   - compact_render_collapsed_large:   < 100µs  (20 reads + 10 searches)
//   - compact_render_measureonly:       < 5µs

func BenchmarkCompact_render_collapsed_small(b *testing.B) {
	c := NewCompactSummary()
	c.ReadCount = 1
	ctx := Context{Cols: 80, Theme: DefaultTheme{}, Wrap: WrapSoft}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = c.Render(ctx)
	}
}

func BenchmarkCompact_render_collapsed_large(b *testing.B) {
	c := NewCompactSummary()
	c.ReadCount = 20
	c.SearchCount = 10
	ctx := Context{Cols: 80, Theme: DefaultTheme{}, Wrap: WrapSoft}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = c.Render(ctx)
	}
}

func BenchmarkCompact_render_measureonly(b *testing.B) {
	c := NewCompactSummary()
	c.ReadCount = 5
	c.SearchCount = 2
	ctx := Context{Cols: 80, MeasureOnly: true, Theme: DefaultTheme{}, Wrap: WrapSoft}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = c.Render(ctx)
	}
}

// spec-1.11 § 4.3 perf targets:
//   - BenchmarkMarkdown_render_paragraph (40-char paragraph):   < 50µs
//   - BenchmarkMarkdown_render_fence_block (10-line fence):    < 100µs
//   - BenchmarkMarkdown_render_complex_doc_uncached:           < 400µs
//   - BenchmarkMarkdown_render_complex_doc_cached:               < 5µs

func BenchmarkMarkdown_render_paragraph(b *testing.B) {
	m := NewMarkdown("The quick brown fox jumps over the lazy dog.")
	ctx := Context{Cols: 80, Theme: DefaultTheme{}, Wrap: WrapSoft}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resetBlockCacheForTest()
		_, _ = m.Render(ctx)
	}
}

func BenchmarkMarkdown_render_fence_block(b *testing.B) {
	m := NewMarkdown("```go\n" +
		"func main() {\n" +
		"    fmt.Println(\"hello\")\n" +
		"    for i := 0; i < 10; i++ {\n" +
		"        x := i * 2\n" +
		"        _ = x\n" +
		"    }\n" +
		"    return\n" +
		"}\n" +
		"```")
	ctx := Context{Cols: 80, Theme: DefaultTheme{}, Wrap: WrapSoft}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resetBlockCacheForTest()
		_, _ = m.Render(ctx)
	}
}

func BenchmarkMarkdown_render_complex_doc_uncached(b *testing.B) {
	m := NewMarkdown("# Heading\n\nFirst paragraph with **bold** and *italic*.\n\n" +
		"Second paragraph with `code` and [link](https://example.com).\n\n" +
		"- list item 1\n- list item 2\n\n" +
		"```go\nfunc x() {}\n```")
	ctx := Context{Cols: 80, Theme: DefaultTheme{}, Wrap: WrapSoft}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resetBlockCacheForTest()
		_, _ = m.Render(ctx)
	}
}

func BenchmarkMarkdown_render_complex_doc_cached(b *testing.B) {
	m := NewMarkdown("# Heading\n\nFirst paragraph with **bold** and *italic*.\n\n" +
		"Second paragraph with `code` and [link](https://example.com).\n\n" +
		"- list item 1\n- list item 2\n\n" +
		"```go\nfunc x() {}\n```")
	ctx := Context{Cols: 80, Theme: DefaultTheme{}, Wrap: WrapSoft}
	// Prime the cache (1 render outside timer).
	resetBlockCacheForTest()
	_, _ = m.Render(ctx)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = m.Render(ctx) // cache hit every iter
	}
}

// spec-1.12 § 4.3 perf targets:
//   - BenchmarkCode_render_highlight_short (10-line Go func):    < 80µs
//   - BenchmarkCode_render_highlight_long (100-line Python):    < 800µs
//   - BenchmarkCode_render_cached (cache hit):                     < 5µs
//   - BenchmarkCode_render_plain_noLang:                          < 60µs

func BenchmarkCode_render_highlight_short(b *testing.B) {
	source := strings.Repeat("func x() { return 42 }\n", 10)
	c := NewCode(source, "go")
	ctx := Context{Cols: 80, Theme: DefaultTheme{}, Wrap: WrapSoft, ColorDepth: 16777216}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resetBlockCacheForTest()
		_, _ = c.Render(ctx)
	}
}

func BenchmarkCode_render_highlight_long(b *testing.B) {
	source := strings.Repeat("def foo(x):\n    return x + 1\n", 50)
	c := NewCode(source, "python")
	ctx := Context{Cols: 80, Theme: DefaultTheme{}, Wrap: WrapSoft, ColorDepth: 16777216}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resetBlockCacheForTest()
		_, _ = c.Render(ctx)
	}
}

func BenchmarkCode_render_cached(b *testing.B) {
	c := NewCode("func main() { fmt.Println(\"hi\") }", "go")
	ctx := Context{Cols: 80, Theme: DefaultTheme{}, Wrap: WrapSoft, ColorDepth: 16777216}
	resetBlockCacheForTest()
	_, _ = c.Render(ctx) // prime
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = c.Render(ctx)
	}
}

func BenchmarkCode_render_plain_noLang(b *testing.B) {
	source := strings.Repeat("plain code line\n", 10)
	c := NewCode(source, "")
	ctx := Context{Cols: 80, Theme: DefaultTheme{}, Wrap: WrapSoft, ColorDepth: 16777216}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resetBlockCacheForTest()
		_, _ = c.Render(ctx)
	}
}

// spec-1.13 § 4.3 perf targets:
//   - BenchmarkDiff_render_small (1 hunk, 5 lines, plain):             < 60µs
//   - BenchmarkDiff_render_large_multihunk (5 hunks, 50 lines, go):    < 500µs
//   - BenchmarkDiff_render_cached (cache hit, bare-lines):             < 5µs
// R2 NIT-1: b.ReportAllocs() enabled so allocs/op + B/op land in default
// `go test -bench` output without -benchmem.

func BenchmarkDiff_render_small(b *testing.B) {
	b.ReportAllocs()
	d := NewDiffFromHunks([]Hunk{{
		OldStart: 1, OldLines: 5, NewStart: 1, NewLines: 5,
		Lines: []LineEntry{
			{Marker: ' ', Text: "a"},
			{Marker: ' ', Text: "b"},
			{Marker: '-', Text: "c"},
			{Marker: '+', Text: "C"},
			{Marker: ' ', Text: "d"},
		},
	}})
	ctx := Context{Cols: 80, Theme: DefaultTheme{}, Wrap: WrapSoft, ColorDepth: 16777216}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resetBlockCacheForTest()
		_, _ = d.Render(ctx)
	}
}

func BenchmarkDiff_render_large_multihunk(b *testing.B) {
	b.ReportAllocs()
	var hunks []Hunk
	for h := 0; h < 5; h++ {
		var lines []LineEntry
		for i := 0; i < 10; i++ {
			marker := ' '
			switch i % 3 {
			case 1:
				marker = '+'
			case 2:
				marker = '-'
			}
			lines = append(lines, LineEntry{Marker: marker, Text: fmt.Sprintf("var x%d = %d", h*10+i, i)})
		}
		hunks = append(hunks, Hunk{OldStart: h*20 + 1, OldLines: 10, NewStart: h*20 + 1, NewLines: 10, Lines: lines})
	}
	d := NewDiffFromHunks(hunks)
	d.BodyLang = "go"
	ctx := Context{Cols: 80, Theme: DefaultTheme{}, Wrap: WrapSoft, ColorDepth: 16777216}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resetBlockCacheForTest()
		_, _ = d.Render(ctx)
	}
}

func BenchmarkDiff_render_cached(b *testing.B) {
	b.ReportAllocs()
	d := NewDiffFromBareLines("+added\n-removed\n context")
	ctx := Context{Cols: 80, Theme: DefaultTheme{}, Wrap: WrapSoft, ColorDepth: 16777216}
	resetBlockCacheForTest()
	_, _ = d.Render(ctx) // prime; per spec § 4.3 LOW-1 NO reset inside loop
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = d.Render(ctx)
	}
}
