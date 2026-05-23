// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package block

import (
	"strings"
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
)

// Test helper: render row to string for assertions.
func rowToString(buf buffer.Buffer, y, cols int) string {
	var b strings.Builder
	for x := 0; x < cols; x++ {
		c := buf.Cell(x, y)
		if c.Ch == 0 {
			b.WriteRune(' ')
		} else {
			b.WriteRune(c.Ch)
		}
	}
	return strings.TrimRight(b.String(), " ")
}

func ctxDefault(cols int) Context {
	return Context{Cols: cols, Theme: DefaultTheme{}, Wrap: WrapSoft}
}

// #1 — Message satisfies RenderNode interface.
func TestMessage_SatisfiesInterface(t *testing.T) {
	var _ RenderNode = Message{}
}

// #2 (痛点 1.5) — Empty placeholder.
func TestMessage_RenderEmptyPlaceholder(t *testing.T) {
	buf, err := Message{Empty: true}.Render(ctxDefault(80))
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	_, rows := buf.Size()
	if rows != 1 {
		t.Fatalf("rows=%d, want 1", rows)
	}
	if got := rowToString(buf, 0, 80); got != "(no output)" {
		t.Fatalf("row 0 = %q, want %q", got, "(no output)")
	}
}

// #3 — Plain text 1 row.
func TestMessage_RenderPlainText(t *testing.T) {
	buf, _ := Message{Text: "hello"}.Render(ctxDefault(80))
	if got := rowToString(buf, 0, 80); got != "hello" {
		t.Fatalf("row 0 = %q, want hello", got)
	}
}

// #4 — Multiline 3 rows.
func TestMessage_RenderMultiline(t *testing.T) {
	buf, _ := Message{Text: "a\nb\nc"}.Render(ctxDefault(80))
	_, rows := buf.Size()
	if rows != 3 {
		t.Fatalf("rows=%d, want 3", rows)
	}
	for i, want := range []string{"a", "b", "c"} {
		if got := rowToString(buf, i, 80); got != want {
			t.Errorf("row %d = %q, want %q", i, got, want)
		}
	}
}

// #5 — Truncated marker.
func TestMessage_RenderTruncatedMarker(t *testing.T) {
	buf, _ := Message{Text: "abc", Truncated: true}.Render(ctxDefault(80))
	row := rowToString(buf, 0, 80)
	if !strings.HasSuffix(row, "abc…") {
		t.Fatalf("row = %q, want suffix abc…", row)
	}
}

// #6 (R2 D1 CRIT-1) — Continued marker same rune as Truncated, distinguished by Style.
func TestMessage_RenderContinuedMarker(t *testing.T) {
	buf, _ := Message{Text: "abc", Continued: true}.Render(ctxDefault(80))
	row := rowToString(buf, 0, 80)
	if !strings.HasSuffix(row, "abc…") {
		t.Fatalf("row = %q, want suffix abc…", row)
	}
	// Verify the marker cell has StyleDimmed (not StyleWarning).
	c := buf.Cell(3, 0)
	if c.Ch != '…' {
		t.Fatalf("Cell(3,0).Ch=%q, want …", c.Ch)
	}
	want := DefaultTheme{}.Style(StyleDimmed)
	if c.St != want {
		t.Errorf("Continued marker Style mismatch: got %+v, want %+v (StyleDimmed)", c.St, want)
	}
}

// #7 — Fence top-level renders code block with lang label.
func TestMessage_RenderFenceTopLevel(t *testing.T) {
	text := "```go\nfn() {}\n```"
	buf, _ := Message{Text: text}.Render(ctxDefault(40))
	_, rows := buf.Size()
	if rows < 2 {
		t.Fatalf("rows=%d, want ≥ 2 (lang label + body)", rows)
	}
	// Lang label row should contain "go".
	if !strings.Contains(rowToString(buf, 0, 40), "go") {
		t.Errorf("row 0 missing 'go': %q", rowToString(buf, 0, 40))
	}
}

// #8 (spec-1.6 R-12 forward) — Fence inside text with intro prose.
func TestMessage_RenderFenceInsideText(t *testing.T) {
	text := "intro\n```go\ncode\n```"
	buf, _ := Message{Text: text}.Render(ctxDefault(40))
	_, rows := buf.Size()
	if rows < 3 {
		t.Fatalf("rows=%d, want ≥ 3 (intro + lang + body)", rows)
	}
	if rowToString(buf, 0, 40) != "intro" {
		t.Errorf("row 0 = %q, want intro", rowToString(buf, 0, 40))
	}
}

// #9 — Fence no lang label.
func TestMessage_RenderFenceNoLang(t *testing.T) {
	text := "```\ncode\n```"
	buf, _ := Message{Text: text}.Render(ctxDefault(20))
	_, rows := buf.Size()
	if rows < 2 {
		t.Fatalf("rows=%d, want ≥ 2", rows)
	}
}

// #10 — 4-backtick fence containing 3 backticks (CommonMark legal).
func TestMessage_Render4BacktickFence(t *testing.T) {
	text := "````go\ninner ``` legal\n````"
	buf, _ := Message{Text: text}.Render(ctxDefault(40))
	_, rows := buf.Size()
	if rows < 2 {
		t.Fatalf("rows=%d, want ≥ 2", rows)
	}
}

// #11 — Unclosed fence runs to EOF.
func TestMessage_RenderUnclosedFence(t *testing.T) {
	text := "```go\nbody no close"
	buf, _ := Message{Text: text}.Render(ctxDefault(40))
	_, rows := buf.Size()
	if rows < 2 {
		t.Fatalf("rows=%d, want ≥ 2", rows)
	}
}

// #12 — Inline single-backtick NOT detected as fence.
func TestMessage_RenderInlineBacktickNotFence(t *testing.T) {
	text := "this is `inline` code"
	buf, _ := Message{Text: text}.Render(ctxDefault(40))
	_, rows := buf.Size()
	if rows != 1 {
		t.Fatalf("rows=%d, want 1 (no fence)", rows)
	}
}

// #13 (CJK) — Soft wrap CJK no cell overflow.
func TestMessage_RenderCJKWrap(t *testing.T) {
	text := "中文段落更长内容啊啊啊啊"
	buf, _ := Message{Text: text}.Render(ctxDefault(10))
	_, rows := buf.Size()
	if rows < 2 {
		t.Fatalf("rows=%d, want ≥ 2 (CJK wrap)", rows)
	}
}

// #14 (R2 D6 HIGH-E) — MeasureOnly returns rows but no cell writes.
func TestMessage_RenderMeasureOnly(t *testing.T) {
	ctx := ctxDefault(80)
	ctx.MeasureOnly = true
	buf, _ := Message{Text: "a\nb\nc"}.Render(ctx)
	_, rows := buf.Size()
	if rows != 3 {
		t.Fatalf("MeasureOnly rows=%d, want 3", rows)
	}
	// No cell writes.
	c := buf.Cell(0, 0)
	if c.Ch != 0 {
		t.Errorf("MeasureOnly should not write cells; got Cell(0,0).Ch=%q", c.Ch)
	}
}

// #15 — Hard wrap policy.
func TestMessage_RenderHardWrapPolicy(t *testing.T) {
	ctx := ctxDefault(5)
	ctx.Wrap = WrapHard
	buf, _ := Message{Text: "abcdefgh"}.Render(ctx)
	_, rows := buf.Size()
	if rows != 2 {
		t.Fatalf("Hard wrap rows=%d, want 2 (abcde + fgh)", rows)
	}
}

// #16 — None wrap policy truncates with "…".
func TestMessage_RenderNoneWrapPolicy(t *testing.T) {
	ctx := ctxDefault(5)
	ctx.Wrap = WrapNone
	buf, _ := Message{Text: "abcdefgh"}.Render(ctx)
	_, rows := buf.Size()
	if rows != 1 {
		t.Fatalf("None wrap rows=%d, want 1", rows)
	}
	row := rowToString(buf, 0, 5)
	if !strings.HasSuffix(row, "…") {
		t.Errorf("None wrap row=%q, want ends with …", row)
	}
}

// #17 — Empty text Text:"" (Empty=false) → 0 rows.
func TestMessage_RenderEmptyText(t *testing.T) {
	buf, _ := Message{Text: ""}.Render(ctxDefault(80))
	_, rows := buf.Size()
	if rows != 0 {
		t.Fatalf("empty Text rows=%d, want 0", rows)
	}
}

// #18 (R2 D6 HIGH-D) — All-fence text renderMixed.
func TestRenderMixed_AllFence(t *testing.T) {
	text := "```go\ncode\n```"
	buf, _ := Message{Text: text}.Render(ctxDefault(20))
	_, rows := buf.Size()
	if rows < 2 {
		t.Fatalf("all-fence rows=%d, want ≥ 2", rows)
	}
}

// #19 (R2 D6) — Trailing prose after last fence.
func TestRenderMixed_TrailingProse(t *testing.T) {
	text := "```go\nbody\n```\nafter fence"
	buf, _ := Message{Text: text}.Render(ctxDefault(40))
	_, rows := buf.Size()
	if rows < 3 {
		t.Fatalf("trailing prose rows=%d, want ≥ 3", rows)
	}
}

// #20 (R2 D6) — Consecutive fences in single Text.
func TestRenderMixed_ConsecutiveFences(t *testing.T) {
	text := "```a\nx\n```\n```b\ny\n```"
	buf, _ := Message{Text: text}.Render(ctxDefault(20))
	_, rows := buf.Size()
	if rows < 4 {
		t.Fatalf("consecutive fences rows=%d, want ≥ 4", rows)
	}
}

// #21 (R2 D6) — Fence-only with Truncated → marker apply to stitched.
func TestRenderMixed_FenceOnlyTruncated(t *testing.T) {
	text := "```go\nbody\n```"
	buf, _ := Message{Text: text, Truncated: true}.Render(ctxDefault(20))
	if buf == nil {
		t.Fatal("buf nil")
	}
}

// #22 (R2 D6 HIGH-E) — MeasureOnly + SoftWrap row parity.
func TestMeasureOnly_SoftWrapRowParity(t *testing.T) {
	ctx := ctxDefault(20)
	text := strings.Repeat("word ", 10) // > 20 cols
	realBuf, _ := Message{Text: text}.Render(ctx)
	_, realRows := realBuf.Size()

	ctx.MeasureOnly = true
	measBuf, _ := Message{Text: text}.Render(ctx)
	_, measRows := measBuf.Size()

	if realRows != measRows {
		t.Fatalf("row parity broken: real=%d, measure=%d", realRows, measRows)
	}
}

// #23 (R2 D6 HIGH-E) — MeasureOnly + fence row parity.
func TestMeasureOnly_FenceRowParity(t *testing.T) {
	ctx := ctxDefault(20)
	text := "intro\n```go\ncode\n```\nafter"
	realBuf, _ := Message{Text: text}.Render(ctx)
	_, realRows := realBuf.Size()

	ctx.MeasureOnly = true
	measBuf, _ := Message{Text: text}.Render(ctx)
	_, measRows := measBuf.Size()

	if realRows != measRows {
		t.Fatalf("fence row parity broken: real=%d, measure=%d", realRows, measRows)
	}
}

// #24 (R2 D6 HIGH-E) — Empty text Text:"" → 0 rows in measureOnlyBuf
// (buffer.NewGrid 不可用 because rejects rows<=0).
func TestMessage_RenderEmptyTextZeroRow(t *testing.T) {
	buf, _ := Message{Text: ""}.Render(ctxDefault(20))
	_, rows := buf.Size()
	if rows != 0 {
		t.Fatalf("empty Text rows=%d, want 0", rows)
	}
}

// #25 — Truncated && Continued exclusive: Truncated wins.
func TestMessage_RenderTruncatedContinuedExclusive(t *testing.T) {
	buf, _ := Message{Text: "abc", Truncated: true, Continued: true}.Render(ctxDefault(80))
	c := buf.Cell(3, 0) // marker position
	want := DefaultTheme{}.Style(StyleWarning)
	if c.St != want {
		t.Errorf("Truncated should win: got Style %+v, want StyleWarning %+v", c.St, want)
	}
}

// #26 (spec-1.7 T-9 HIGH-1) — renderMixed marker tail overflow:
// fence-first + prose last-row exactly cols wide + Truncated must
// produce +1 row (marker on new row), NOT overwrite last content cell.
func TestMessage_RenderMixedFenceFirstFullProseLastRowTruncated(t *testing.T) {
	// "abcde" exactly cols=5 wide on last prose row (no trailing space).
	text := "```go\npackage main\n```\nabcde"
	buf, _ := Message{Text: text, Truncated: true}.Render(ctxWithCols(5))
	cols, rows := buf.Size()
	if cols != 5 {
		t.Fatalf("expected cols=5, got %d", cols)
	}
	// Expected layout: fence (2 rows: lang label + body) + prose (1 row "abcde") + marker on +1 row.
	// At minimum the last row must contain just the marker "…", not "abcd…" overlap.
	lastRow := rows - 1
	// Marker '…' must be at lastRow col 0 (or some position with theme StyleWarning).
	found := false
	for x := 0; x < cols; x++ {
		c := buf.Cell(x, lastRow)
		if c.Ch == '…' && c.St == (DefaultTheme{}).Style(StyleWarning) {
			found = true
			break
		}
	}
	if !found {
		// Diagnostic: dump last row + the row above it.
		dumpRow := func(y int) string {
			var b strings.Builder
			for x := 0; x < cols; x++ {
				c := buf.Cell(x, y)
				if c.Ch == 0 || c.Ch == ' ' {
					b.WriteRune(' ')
				} else {
					b.WriteRune(c.Ch)
				}
			}
			return b.String()
		}
		prev := ""
		if lastRow >= 1 {
			prev = dumpRow(lastRow - 1)
		}
		t.Errorf("marker '…' StyleWarning not found on last row %d\nrow-1: %q\nlast:  %q",
			lastRow, prev, dumpRow(lastRow))
	}
}

// #27 (spec-1.7 T-9 HIGH-2) — renderCodeBlock body advances x by RuneWidth
// for wide-rune (CJK) chars; no clobber from clearWideOverlap.
func TestMessage_RenderCodeFenceCJKBody(t *testing.T) {
	text := "```go\n中文测试\n```"
	buf, _ := Message{Text: text}.Render(ctxWithCols(20))
	// Body row is index 1 (row 0 = lang label "─── go ───").
	// "中文测试" = 4 wide runes × 2 cells = 8 cells; starting at x=1 (1-cell left padding).
	// Expected: Cell(1)='中', Cell(3)='文', Cell(5)='测', Cell(7)='试'.
	wantChars := []rune{'中', '文', '测', '试'}
	wantXs := []int{1, 3, 5, 7}
	for i, want := range wantChars {
		c := buf.Cell(wantXs[i], 1)
		if c.Ch != want {
			t.Errorf("body wide rune #%d: at col %d got Ch=%q (raw %U), want %q",
				i, wantXs[i], c.Ch, c.Ch, want)
		}
	}
}

func ctxWithCols(cols int) Context {
	return Context{Cols: cols, Rows: 24, Wrap: WrapSoft}
}
