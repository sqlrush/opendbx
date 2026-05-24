// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File markdown_test.go — spec-1.11 D-7 unit test suite per § 4.1 (T1-1
// to T1-38). Covers block elements (heading / paragraph / list / blockquote
// / fence / hr / table) + inline (bold / italic / code / strikethrough
// literal / link visible-fallback) + edge cases (empty / CJK / wrap) +
// cache (hit / miss / eviction / themeKey invalidate / >256KB skip) +
// concurrent race + invalid UTF-8.

package block

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
	"github.com/sqlrush/opendbx/internal/app/cli/render/width"
)

func ctxMd(cols int) Context {
	return Context{Cols: cols, Rows: 24, Wrap: WrapSoft}
}

// mustRenderMd surfaces Render errors as fatal (rule 7).
func mustRenderMd(t *testing.T, m Markdown, ctx Context) buffer.Buffer {
	t.Helper()
	buf, err := m.Render(ctx)
	if err != nil {
		t.Fatalf("Markdown.Render: unexpected error: %v", err)
	}
	return buf
}

// rowText reconstructs a row's text content from cells.
func rowText(buf buffer.Buffer, y int) string {
	cols, _ := buf.Size()
	var b strings.Builder
	for x := 0; x < cols; x++ {
		c := buf.Cell(x, y)
		if c.Ch == 0 || buffer.IsContinuation(c) {
			continue
		}
		b.WriteRune(c.Ch)
	}
	return b.String()
}

// ---- T1-1..T1-3: edge / paragraph ----

func TestMarkdown_T1_Empty(t *testing.T) {
	resetMarkdownCacheForTest()
	m := NewMarkdown("")
	buf := mustRenderMd(t, m, ctxMd(80))
	_, rows := buf.Size()
	if rows != 0 {
		t.Errorf("empty source: want 0 rows, got %d", rows)
	}
}

func TestMarkdown_T2_WhitespaceOnly(t *testing.T) {
	resetMarkdownCacheForTest()
	m := NewMarkdown("   \n  \n")
	buf := mustRenderMd(t, m, ctxMd(80))
	_, rows := buf.Size()
	if rows != 0 {
		t.Errorf("whitespace-only: want 0 rows, got %d", rows)
	}
}

func TestMarkdown_T3_SingleParagraph(t *testing.T) {
	resetMarkdownCacheForTest()
	m := NewMarkdown("hello world")
	buf := mustRenderMd(t, m, ctxMd(80))
	if !strings.Contains(rowText(buf, 0), "hello world") {
		t.Errorf("paragraph: want 'hello world', got %q", rowText(buf, 0))
	}
}

// ---- T1-4..T1-5: headings ----

func TestMarkdown_T4_HeadingH1(t *testing.T) {
	resetMarkdownCacheForTest()
	m := NewMarkdown("# Title")
	buf := mustRenderMd(t, m, ctxMd(80))
	text := rowText(buf, 0)
	if !strings.Contains(text, "Title") {
		t.Errorf("H1: want 'Title', got %q", text)
	}
	// R3 baseline: no prefix glyph
	if strings.HasPrefix(text, "#") {
		t.Errorf("H1: must NOT have '#' prefix glyph (R3 CC baseline), got %q", text)
	}
}

func TestMarkdown_T5_HeadingsH2toH6(t *testing.T) {
	resetMarkdownCacheForTest()
	src := "## H2\n### H3\n#### H4\n##### H5\n###### H6"
	m := NewMarkdown(src)
	buf := mustRenderMd(t, m, ctxMd(80))
	_, rows := buf.Size()
	if rows < 5 {
		t.Errorf("H2-H6: want ≥5 rows, got %d", rows)
	}
}

// ---- T1-6..T1-9: lists ----

func TestMarkdown_T6_UnorderedListBullet(t *testing.T) {
	resetMarkdownCacheForTest()
	m := NewMarkdown("- item1\n- item2")
	buf := mustRenderMd(t, m, ctxMd(80))
	// R3 baseline: "- " prefix (not "• ")
	r0 := rowText(buf, 0)
	if !strings.HasPrefix(r0, "- ") {
		t.Errorf("unordered: want '- ' prefix (R3 CC), got %q", r0)
	}
	if strings.Contains(r0, "•") {
		t.Errorf("unordered: must NOT use '•' U+2022 (R3 CC), got %q", r0)
	}
}

func TestMarkdown_T7_OrderedListNumbered(t *testing.T) {
	resetMarkdownCacheForTest()
	m := NewMarkdown("1. one\n2. two")
	buf := mustRenderMd(t, m, ctxMd(80))
	if !strings.HasPrefix(rowText(buf, 0), "1. ") {
		t.Errorf("ordered: want '1. ' prefix, got %q", rowText(buf, 0))
	}
	if !strings.HasPrefix(rowText(buf, 1), "2. ") {
		t.Errorf("ordered: want '2. ' prefix on row 1, got %q", rowText(buf, 1))
	}
}

func TestMarkdown_T8_NestedList2Level(t *testing.T) {
	resetMarkdownCacheForTest()
	m := NewMarkdown("- a\n  - a1\n- b")
	buf := mustRenderMd(t, m, ctxMd(80))
	_, rows := buf.Size()
	if rows < 3 {
		t.Errorf("nested 2-level: want ≥3 rows, got %d", rows)
	}
	// a1 should be indented (have leading whitespace).
	if !strings.Contains(rowText(buf, 1), "a1") {
		t.Errorf("nested row text: %q", rowText(buf, 1))
	}
}

func TestMarkdown_T9_NestedList3Level(t *testing.T) {
	resetMarkdownCacheForTest()
	m := NewMarkdown("- a\n  - a1\n    - a1.1")
	buf := mustRenderMd(t, m, ctxMd(80))
	_, rows := buf.Size()
	if rows < 3 {
		t.Errorf("nested 3-level: want ≥3 rows, got %d", rows)
	}
}

// ---- T1-10..T1-11: blockquote ----

func TestMarkdown_T10_BlockquoteSingleLine(t *testing.T) {
	resetMarkdownCacheForTest()
	m := NewMarkdown("> quoted")
	buf := mustRenderMd(t, m, ctxMd(80))
	r0 := rowText(buf, 0)
	if !strings.HasPrefix(r0, "▎ ") {
		t.Errorf("blockquote: want '▎ ' U+258E rail prefix (R3 CC), got %q", r0)
	}
	if !strings.Contains(r0, "quoted") {
		t.Errorf("blockquote: missing content 'quoted', got %q", r0)
	}
}

func TestMarkdown_BlockquoteInlineSpanByteShift(t *testing.T) {
	resetMarkdownCacheForTest()
	ctx := ctxMd(80)
	ctx.Theme = DefaultTheme{}
	m := NewMarkdown("> **bold**")
	buf := mustRenderMd(t, m, ctx)
	if got := rowText(buf, 0); !strings.HasPrefix(got, "▎ bold") {
		t.Fatalf("blockquote text: got %q", got)
	}
	for _, x := range []int{2, 3, 4, 5} {
		if c := buf.Cell(x, 0); !c.St.Bold {
			t.Fatalf("blockquote bold span at x=%d: want bold cell, got Ch=%q style=%#v row=%q", x, c.Ch, c.St, rowText(buf, 0))
		}
	}
}

// TestMarkdown_BlockquoteFence — R6 CRIT-1 regression test (claude path
// 1/3 catch): fence-in-blockquote was silently dropping "▎ " rail
// because buildBuffer cells-path bypassed mutated rowSpec.text. Fix:
// rowSpec.prefix is rendered separately for both text and cells paths.
func TestMarkdown_BlockquoteFence(t *testing.T) {
	resetMarkdownCacheForTest()
	m := NewMarkdown("> ```go\n> code line\n> ```")
	buf := mustRenderMd(t, m, ctxMd(40))
	_, rows := buf.Size()
	if rows < 2 {
		t.Fatalf("fence-in-blockquote: want ≥2 rows, got %d", rows)
	}
	// Every emitted row must carry the rail prefix on cell 0.
	for y := 0; y < rows; y++ {
		c0 := buf.Cell(0, y).Ch
		if c0 != '▎' {
			t.Errorf("row %d: want '▎' rail at cell 0 (CRIT-1 R6 fix), got %q (row text: %q)",
				y, c0, rowText(buf, y))
		}
	}
}

// TestMarkdown_BlockquoteNested — R6 MED-3 + CRIT-1 sibling: nested
// blockquote should stack rails ("▎ ▎ inner").
func TestMarkdown_BlockquoteNested(t *testing.T) {
	resetMarkdownCacheForTest()
	m := NewMarkdown("> outer\n>\n> > inner")
	buf := mustRenderMd(t, m, ctxMd(40))
	_, rows := buf.Size()
	if rows < 2 {
		t.Fatalf("nested blockquote: want ≥2 rows, got %d", rows)
	}
	// Find the row containing "inner" — it should have stacked rails.
	var innerRow int = -1
	for y := 0; y < rows; y++ {
		if strings.Contains(rowText(buf, y), "inner") {
			innerRow = y
			break
		}
	}
	if innerRow < 0 {
		t.Fatalf("nested blockquote: missing 'inner' row; rendered: %v", dumpRows(buf, rows))
	}
	text := rowText(buf, innerRow)
	if !strings.HasPrefix(text, "▎ ▎ ") {
		t.Errorf("nested blockquote inner: want '▎ ▎ ' stacked rail prefix, got %q", text)
	}
}

// dumpRows is a test helper for nested-blockquote debugging.
func dumpRows(buf buffer.Buffer, rows int) []string {
	out := make([]string, rows)
	for y := 0; y < rows; y++ {
		out[y] = rowText(buf, y)
	}
	return out
}

func TestMarkdown_T11_BlockquoteMultiLine(t *testing.T) {
	resetMarkdownCacheForTest()
	// CommonMark: two adjacent `>` lines form a single Paragraph in
	// blockquote. We just verify the rail appears.
	m := NewMarkdown("> line1\n> line2")
	buf := mustRenderMd(t, m, ctxMd(80))
	_, rows := buf.Size()
	if rows < 1 {
		t.Fatalf("blockquote multi: want ≥1 row, got %d", rows)
	}
	if !strings.HasPrefix(rowText(buf, 0), "▎ ") {
		t.Errorf("blockquote multi: row 0 missing rail, got %q", rowText(buf, 0))
	}
}

// ---- T1-12..T1-14: fence ----

func TestMarkdown_T12_FenceNoLang(t *testing.T) {
	resetMarkdownCacheForTest()
	m := NewMarkdown("```\ncode_line\n```")
	buf := mustRenderMd(t, m, ctxMd(80))
	_, rows := buf.Size()
	if rows == 0 {
		t.Errorf("fence: want >0 rows via spec-1.7 renderCodeBlock delegate, got 0")
	}
}

func TestMarkdown_FencePreservesRenderCodeBlockStyles(t *testing.T) {
	resetMarkdownCacheForTest()
	ctx := ctxMd(24)
	ctx.Theme = DefaultTheme{}
	m := NewMarkdown("```go\ncode\n```")
	buf := mustRenderMd(t, m, ctx)
	wantLabel := DefaultTheme{}.Style(StyleLangLabel)
	if got := buf.Cell(0, 0).St; got != wantLabel {
		t.Fatalf("fence label style: want %#v, got %#v", wantLabel, got)
	}
	wantCodeBg := DefaultTheme{}.Style(StyleCodeBg).BG
	if got := buf.Cell(0, 1).St.BG; got != wantCodeBg {
		t.Fatalf("fence body bg at padding cell: want %#v, got %#v", wantCodeBg, got)
	}
	if c := buf.Cell(1, 1); c.Ch != 'c' || c.St.BG != wantCodeBg {
		t.Fatalf("fence body code cell: want 'c' with bg %#v, got Ch=%q style=%#v", wantCodeBg, c.Ch, c.St)
	}
}

func TestMarkdown_T13_FenceWithLangGo(t *testing.T) {
	resetMarkdownCacheForTest()
	m := NewMarkdown("```go\nfunc main()\n```")
	buf := mustRenderMd(t, m, ctxMd(80))
	_, rows := buf.Size()
	if rows == 0 {
		t.Errorf("fence with lang: want >0 rows, got 0")
	}
}

func TestMarkdown_T14_FenceEmptyBody(t *testing.T) {
	resetMarkdownCacheForTest()
	m := NewMarkdown("```go\n```")
	buf := mustRenderMd(t, m, ctxMd(80))
	// Empty fence — spec-1.7 contract; just verify no panic.
	_, _ = buf.Size()
}

// ---- T1-15: HR ----

func TestMarkdown_T15_HR(t *testing.T) {
	resetMarkdownCacheForTest()
	m := NewMarkdown("---")
	buf := mustRenderMd(t, m, ctxMd(80))
	r0 := rowText(buf, 0)
	if !strings.Contains(r0, "---") {
		t.Errorf("HR: want '---' (R3 CC, not '─'), got %q", r0)
	}
	if strings.Contains(r0, "─") {
		t.Errorf("HR: must NOT use '─' U+2500 (R3 CC), got %q", r0)
	}
}

// ---- T1-16..T1-17: tables ----

func TestMarkdown_T16_Table2x2(t *testing.T) {
	resetMarkdownCacheForTest()
	m := NewMarkdown("| a | b |\n|---|---|\n| 1 | 2 |")
	buf := mustRenderMd(t, m, ctxMd(80))
	_, rows := buf.Size()
	if rows < 4 {
		t.Errorf("table 2x2: want ≥4 rows (sep+header+sep+data), got %d", rows)
	}
}

func TestMarkdown_TableCJKUsesDisplayWidth(t *testing.T) {
	resetMarkdownCacheForTest()
	m := NewMarkdown("| 中 |\n|---|\n| a |")
	buf := mustRenderMd(t, m, ctxMd(80))
	_, rows := buf.Size()
	if rows < 4 {
		t.Fatalf("table rows: want >=4, got %d", rows)
	}
	topWidth := width.Width(rowText(buf, 0))
	headerWidth := width.Width(rowText(buf, 1))
	dataWidth := width.Width(rowText(buf, 3))
	if headerWidth != topWidth || dataWidth != topWidth {
		t.Fatalf("table visual widths mismatch: top=%d header=%d data=%d rows=%q / %q / %q", topWidth, headerWidth, dataWidth, rowText(buf, 0), rowText(buf, 1), rowText(buf, 3))
	}
}

func TestMarkdown_T17_Table3x3(t *testing.T) {
	resetMarkdownCacheForTest()
	src := "| a | b | c |\n|---|---|---|\n| 1 | 2 | 3 |\n| 4 | 5 | 6 |\n| 7 | 8 | 9 |"
	m := NewMarkdown(src)
	buf := mustRenderMd(t, m, ctxMd(80))
	_, rows := buf.Size()
	if rows < 5 {
		t.Errorf("table 3x3: want ≥5 rows, got %d", rows)
	}
}

// ---- T1-18: mixed sequence ----

func TestMarkdown_T18_MixedBlockSequence(t *testing.T) {
	resetMarkdownCacheForTest()
	src := "# Title\n\nParagraph\n\n- item"
	m := NewMarkdown(src)
	buf := mustRenderMd(t, m, ctxMd(80))
	_, rows := buf.Size()
	if rows < 3 {
		t.Errorf("mixed: want ≥3 rows (heading+para+list), got %d", rows)
	}
}

// ---- T1-19..T1-25: inline ----

func TestMarkdown_T19_BoldInline(t *testing.T) {
	resetMarkdownCacheForTest()
	ctx := ctxMd(80)
	ctx.Theme = DefaultTheme{}
	m := NewMarkdown("**bold**")
	buf := mustRenderMd(t, m, ctx)
	if !strings.Contains(rowText(buf, 0), "bold") {
		t.Errorf("bold: missing 'bold', got %q", rowText(buf, 0))
	}
	// R6 MED-1: assert cell style not just text presence.
	for _, x := range []int{0, 1, 2, 3} {
		if c := buf.Cell(x, 0); !c.St.Bold {
			t.Errorf("bold cell at x=%d: want Bold style, got Ch=%q St=%#v", x, c.Ch, c.St)
		}
	}
}

func TestMarkdown_T20_ItalicInline(t *testing.T) {
	resetMarkdownCacheForTest()
	ctx := ctxMd(80)
	ctx.Theme = DefaultTheme{}
	m := NewMarkdown("*italic*")
	buf := mustRenderMd(t, m, ctx)
	if !strings.Contains(rowText(buf, 0), "italic") {
		t.Errorf("italic: missing 'italic', got %q", rowText(buf, 0))
	}
	// R6 MED-1: assert cell style not just text presence.
	for _, x := range []int{0, 1, 2, 3, 4, 5} {
		if c := buf.Cell(x, 0); !c.St.Italic {
			t.Errorf("italic cell at x=%d: want Italic style, got Ch=%q St=%#v", x, c.Ch, c.St)
		}
	}
}

func TestMarkdown_T21_InlineCode(t *testing.T) {
	resetMarkdownCacheForTest()
	ctx := ctxMd(80)
	ctx.Theme = DefaultTheme{}
	m := NewMarkdown("`code`")
	buf := mustRenderMd(t, m, ctx)
	r0 := rowText(buf, 0)
	if !strings.Contains(r0, "code") {
		t.Errorf("inline code: missing 'code', got %q", r0)
	}
	// R6 HIGH-1: backticks are markdown syntax, not rendered output.
	if strings.Contains(r0, "`") {
		t.Errorf("inline code: must NOT contain literal backtick (R6 HIGH-1 CC formatToken contract), got %q", r0)
	}
}

func TestMarkdown_T22_StrikethroughLiteral(t *testing.T) {
	resetMarkdownCacheForTest()
	// R3 CC baseline: strikethrough tokenizer is disabled; ~~text~~
	// must stay literal (no strike style).
	m := NewMarkdown("~~strike~~")
	buf := mustRenderMd(t, m, ctxMd(80))
	r0 := rowText(buf, 0)
	if !strings.Contains(r0, "~~strike~~") {
		t.Errorf("strikethrough literal (R3 CC): want '~~strike~~' preserved, got %q", r0)
	}
}

func TestMarkdown_T23_Link(t *testing.T) {
	resetMarkdownCacheForTest()
	m := NewMarkdown("[text](https://example.com)")
	buf := mustRenderMd(t, m, ctxMd(80))
	r0 := rowText(buf, 0)
	if !strings.Contains(r0, "text") {
		t.Errorf("link: missing visible text, got %q", r0)
	}
	// R3 ❌-8 fallback: URL visible in row (no OSC8 metadata)
	if !strings.Contains(r0, "example.com") {
		t.Errorf("link: missing URL fallback, got %q", r0)
	}
}

func TestMarkdown_T24_Autolink(t *testing.T) {
	resetMarkdownCacheForTest()
	m := NewMarkdown("<https://example.com>")
	buf := mustRenderMd(t, m, ctxMd(80))
	if !strings.Contains(rowText(buf, 0), "example.com") {
		t.Errorf("autolink: missing URL, got %q", rowText(buf, 0))
	}
}

func TestMarkdown_T25_MixedInlineInParagraph(t *testing.T) {
	resetMarkdownCacheForTest()
	m := NewMarkdown("Hello **bold** and *italic* and `code`")
	buf := mustRenderMd(t, m, ctxMd(80))
	r0 := rowText(buf, 0)
	for _, want := range []string{"Hello", "bold", "italic", "code"} {
		if !strings.Contains(r0, want) {
			t.Errorf("mixed inline: missing %q, got %q", want, r0)
		}
	}
}

// ---- T1-26..T1-27: wrap + CJK ----

func TestMarkdown_T26_LongParagraphWrap(t *testing.T) {
	resetMarkdownCacheForTest()
	long := strings.Repeat("word ", 500) // 2500 chars
	m := NewMarkdown(long)
	buf := mustRenderMd(t, m, ctxMd(80))
	_, rows := buf.Size()
	if rows < 30 {
		t.Errorf("long paragraph wrap: want ≥30 rows at cols=80, got %d", rows)
	}
}

func TestMarkdown_T27_CJK(t *testing.T) {
	resetMarkdownCacheForTest()
	m := NewMarkdown("# 中文标题\n\n中文段落内容")
	buf := mustRenderMd(t, m, ctxMd(80))
	r0 := rowText(buf, 0)
	if !strings.ContainsRune(r0, '中') {
		t.Errorf("CJK heading: missing '中', got %q", r0)
	}
}

// ---- T1-28..T1-29: ctx boundaries ----

func TestMarkdown_T28_ColsZero(t *testing.T) {
	resetMarkdownCacheForTest()
	m := NewMarkdown("# Title")
	buf := mustRenderMd(t, m, Context{Cols: 0})
	cols, rows := buf.Size()
	if cols != 0 || rows != 0 {
		t.Errorf("Cols=0: want (0,0), got (%d,%d)", cols, rows)
	}
}

func TestMarkdown_T29_MeasureOnly(t *testing.T) {
	resetMarkdownCacheForTest()
	m := NewMarkdown("# Title\n\nWorld paragraph here")
	ctx := ctxMd(80)
	ctx.MeasureOnly = true
	buf := mustRenderMd(t, m, ctx)
	_, rows := buf.Size()
	if rows == 0 {
		t.Errorf("MeasureOnly: want row count, got 0")
	}
	// Confirm no cell writes (cells must be zero).
	if c := buf.Cell(0, 0); c.Ch != 0 {
		t.Errorf("MeasureOnly: want zero cell at (0,0), got Ch=%q", c.Ch)
	}
}

// ---- T1-30..T1-33b: cache ----

func TestMarkdown_T30_CacheHit(t *testing.T) {
	resetMarkdownCacheForTest()
	src := "# Cached"
	m := NewMarkdown(src)
	buf1 := mustRenderMd(t, m, ctxMd(80))
	buf2 := mustRenderMd(t, m, ctxMd(80))
	// I-8 immutability contract: cache returns same Buffer reference.
	if fmt.Sprintf("%p", buf1) != fmt.Sprintf("%p", buf2) {
		t.Errorf("cache hit (I-8): want same Buffer reference, got different (%p vs %p)", buf1, buf2)
	}
}

func TestMarkdown_T31_CacheEvictionAtCapPlus1(t *testing.T) {
	resetMarkdownCacheForTest()
	// Render markdownCacheCap+1 distinct sources.
	for i := 0; i < markdownCacheCap+1; i++ {
		m := NewMarkdown(fmt.Sprintf("# Source %d", i))
		_, err := m.Render(ctxMd(80))
		if err != nil {
			t.Fatalf("eviction iter %d error: %v", i, err)
		}
	}
	if markdownCache.Len() > markdownCacheCap {
		t.Errorf("eviction: want Len ≤ cap=%d, got %d", markdownCacheCap, markdownCache.Len())
	}
}

func TestMarkdown_T32_VerboseChangeInvalidate(t *testing.T) {
	resetMarkdownCacheForTest()
	src := "# Same"
	m := NewMarkdown(src)
	ctxA := ctxMd(80)
	ctxA.Verbose = false
	buf1 := mustRenderMd(t, m, ctxA)
	ctxB := ctxMd(80)
	ctxB.Verbose = true
	buf2 := mustRenderMd(t, m, ctxB)
	if fmt.Sprintf("%p", buf1) == fmt.Sprintf("%p", buf2) {
		t.Errorf("verbose change: want different cache entry, got same Buffer ref")
	}
}

func TestMarkdown_T33_ColsChangeInvalidate(t *testing.T) {
	resetMarkdownCacheForTest()
	src := "# Same"
	m := NewMarkdown(src)
	buf1 := mustRenderMd(t, m, ctxMd(80))
	buf2 := mustRenderMd(t, m, ctxMd(120))
	if fmt.Sprintf("%p", buf1) == fmt.Sprintf("%p", buf2) {
		t.Errorf("cols change: want different cache entry, got same Buffer ref")
	}
}

// fakeTheme2 / fakeTheme3 embed DefaultTheme but override
// MarkdownCacheKey() to verify cache key invalidation on theme switch.
type fakeTheme2 struct{ DefaultTheme }

func (fakeTheme2) MarkdownCacheKey() string { return "fake2" }

type fakeTheme3 struct{ DefaultTheme }

func (fakeTheme3) MarkdownCacheKey() string { return "fake3" }

func TestMarkdown_T33b_ThemeKeyChangeInvalidate(t *testing.T) {
	resetMarkdownCacheForTest()
	src := "# Themed"
	m := NewMarkdown(src)
	ctxA := ctxMd(80)
	ctxA.Theme = fakeTheme2{}
	buf1 := mustRenderMd(t, m, ctxA)
	ctxB := ctxMd(80)
	ctxB.Theme = fakeTheme3{}
	buf2 := mustRenderMd(t, m, ctxB)
	if fmt.Sprintf("%p", buf1) == fmt.Sprintf("%p", buf2) {
		t.Errorf("theme change: want different cache entry, got same Buffer ref")
	}
}

// ---- T1-34: raw HTML drop ----

func TestMarkdown_T34_RawHTMLDrop(t *testing.T) {
	resetMarkdownCacheForTest()
	m := NewMarkdown("<div>x</div>")
	buf := mustRenderMd(t, m, ctxMd(80))
	_, rows := buf.Size()
	// R3 CC baseline: raw HTML block dropped, not rendered.
	// (May result in 0 rows for pure-HTML input.)
	for y := 0; y < rows; y++ {
		text := rowText(buf, y)
		if strings.Contains(text, "<div>") {
			t.Errorf("raw HTML drop (R3 CC ❌-4 I-5): row %d should not contain '<div>', got %q", y, text)
		}
	}
}

// ---- T1-35: backslash escape ----

func TestMarkdown_T35_BackslashEscape(t *testing.T) {
	resetMarkdownCacheForTest()
	m := NewMarkdown(`\*not italic\*`)
	buf := mustRenderMd(t, m, ctxMd(80))
	r0 := rowText(buf, 0)
	if !strings.Contains(r0, "*not italic*") {
		t.Errorf("backslash escape: want literal '*not italic*', got %q", r0)
	}
}

// ---- T1-36: concurrent race ----

func TestMarkdown_T36_ConcurrentRenderRace(t *testing.T) {
	resetMarkdownCacheForTest()
	src := "# Concurrent\n\n- item1\n- item2"
	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				m := NewMarkdown(src)
				_, _ = m.Render(ctxMd(80))
			}
		}()
	}
	wg.Wait()
	// If we got here without panic / data race (go test -race), pass.
	if markdownCache.Len() == 0 {
		t.Errorf("concurrent: cache should have entries after 400 renders")
	}
}

// ---- T1-37: 256KB boundary ----

func TestMarkdown_T37_256KBSourceBoundary(t *testing.T) {
	resetMarkdownCacheForTest()
	// At exactly the cap: cacheable.
	atCap := strings.Repeat("a", markdownCacheMaxSourceBytes)
	m1 := NewMarkdown(atCap)
	buf1a := mustRenderMd(t, m1, ctxMd(80))
	buf1b := mustRenderMd(t, m1, ctxMd(80))
	if fmt.Sprintf("%p", buf1a) != fmt.Sprintf("%p", buf1b) {
		t.Errorf("at cap (262144 bytes): want cacheable + same Buffer ref, got different")
	}
	// First over-cap: NOT cacheable.
	resetMarkdownCacheForTest()
	overCap := strings.Repeat("a", markdownCacheMaxSourceBytes+1)
	m2 := NewMarkdown(overCap)
	buf2a := mustRenderMd(t, m2, ctxMd(80))
	buf2b := mustRenderMd(t, m2, ctxMd(80))
	if fmt.Sprintf("%p", buf2a) == fmt.Sprintf("%p", buf2b) {
		t.Errorf("over cap (262145 bytes): want NOT cacheable (different Buffer ref), got same")
	}
}

// ---- T1-12b: indented code block (4-space) ----

func TestMarkdown_T12b_IndentedCodeBlock(t *testing.T) {
	resetMarkdownCacheForTest()
	// CommonMark: 4-space indent = code block, no fence.
	m := NewMarkdown("    indented\n    code line")
	buf := mustRenderMd(t, m, ctxMd(80))
	_, rows := buf.Size()
	if rows == 0 {
		t.Errorf("indented code block: want >0 rows, got 0")
	}
}

// ---- T1-38: invalid UTF-8 ----

func TestMarkdown_T38_InvalidUTF8(t *testing.T) {
	resetMarkdownCacheForTest()
	m := NewMarkdown(string([]byte{0xff, 0xfe, ' ', 'g', 'a', 'r', 'b', 'a', 'g', 'e'}))
	// Just verify no panic.
	_, err := m.Render(ctxMd(80))
	if err != nil {
		t.Errorf("invalid UTF-8: unexpected error: %v", err)
	}
}

// ---- themeCacheKey contract (R3.1) ----

func TestThemeCacheKey_NilDefault(t *testing.T) {
	if got := themeCacheKey(nil); got != "default" {
		t.Errorf("nil theme: want 'default', got %q", got)
	}
}

func TestThemeCacheKey_OptInInterface(t *testing.T) {
	if got := themeCacheKey(fakeTheme2{}); got != "fake2" {
		t.Errorf("MarkdownCacheKey() interface: want 'fake2', got %q", got)
	}
}

func TestThemeCacheKey_TypeNameFallback(t *testing.T) {
	got := themeCacheKey(DefaultTheme{})
	if !strings.Contains(got, "DefaultTheme") {
		t.Errorf("type-name fallback: want contain 'DefaultTheme', got %q", got)
	}
}
