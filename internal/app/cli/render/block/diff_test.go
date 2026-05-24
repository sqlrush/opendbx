// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File diff_test.go — spec-1.13 D-8 unit test suite per § 4.1 (T1-1
// to T1-35). Covers Hunk ctors / marker classification / gutter compute
// / hunk header / `-` line CC parity skip-highlight / cache invalidation
// / fence dispatch / parser hit/miss/malformed / ColorDepth downgrade
// / ctx.Cols=0 + MeasureOnly fast paths.

package block

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
)

func ctxDiff(cols int) Context {
	return Context{
		Cols:       cols,
		Theme:      DefaultTheme{},
		Wrap:       WrapSoft,
		ColorDepth: 16777216,
	}
}

// ---- T1-1..T1-5: ctors + basic structure ----

func TestDiff_T1_EmptyHunks(t *testing.T) {
	resetBlockCacheForTest()
	d := NewDiffFromHunks(nil)
	buf, err := d.Render(ctxDiff(80))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	_, rows := buf.Size()
	if rows != 0 {
		t.Errorf("empty hunks: want 0 rows, got %d", rows)
	}
}

func TestDiff_T2_SingleHunkSingleLine(t *testing.T) {
	resetBlockCacheForTest()
	d := NewDiffFromHunks([]Hunk{{
		OldStart: 1, OldLines: 0, NewStart: 1, NewLines: 1,
		Lines: []LineEntry{{Marker: '+', Text: "x"}},
	}})
	buf, _ := d.Render(ctxDiff(80))
	_, rows := buf.Size()
	if rows != 2 { // header + 1 body
		t.Errorf("single hunk: want 2 rows, got %d", rows)
	}
}

func TestDiff_T3_MultiLineHunk(t *testing.T) {
	resetBlockCacheForTest()
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
	buf, _ := d.Render(ctxDiff(80))
	_, rows := buf.Size()
	if rows != 6 { // header + 5 body
		t.Errorf("5-line hunk: want 6 rows, got %d", rows)
	}
}

func TestDiff_T4_MultiHunk(t *testing.T) {
	resetBlockCacheForTest()
	d := NewDiffFromHunks([]Hunk{
		{OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 1,
			Lines: []LineEntry{{Marker: '+', Text: "a"}}},
		{OldStart: 10, OldLines: 1, NewStart: 10, NewLines: 1,
			Lines: []LineEntry{{Marker: '-', Text: "b"}}},
	})
	buf, _ := d.Render(ctxDiff(80))
	_, rows := buf.Size()
	if rows != 4 { // 2 headers + 2 body
		t.Errorf("2 hunks: want 4 rows, got %d", rows)
	}
}

func TestDiff_T5_BareLinesNoGutter(t *testing.T) {
	resetBlockCacheForTest()
	d := NewDiffFromBareLines("+a\n-b")
	buf, _ := d.Render(ctxDiff(80))
	_, rows := buf.Size()
	if rows != 2 { // no @@ header in bare-lines mode
		t.Errorf("bare-lines: want 2 rows (no header), got %d", rows)
	}
	// Row 0 cell 0 should be '+' (no gutter prefix).
	if c := buf.Cell(0, 0).Ch; c != '+' {
		t.Errorf("bare-lines row 0 cell 0: want '+', got %q", c)
	}
}

// ---- T1-6..T1-8: prefix style per marker (R3 HIGH-1 R2 CRIT-1 ★A) ----

func TestDiff_T6_MarkerAddedPrefixStyle(t *testing.T) {
	resetBlockCacheForTest()
	d := NewDiffFromBareLines("+foo")
	buf, _ := d.Render(ctxDiff(80))
	// prefix cell 0 should have StyleDiffAdded style.
	wantStyle := DefaultTheme{}.Style(StyleDiffAdded)
	got := buf.Cell(0, 0).St
	if got != wantStyle {
		t.Errorf("'+' prefix style: want %#v, got %#v", wantStyle, got)
	}
}

func TestDiff_T7_MarkerRemovedSkipHighlight(t *testing.T) {
	resetBlockCacheForTest()
	d := NewDiffFromBareLines("-foo")
	d.BodyLang = "go"
	buf, _ := d.Render(ctxDiff(80))
	// R3 CRIT-2 ★A: `-` line body cells use StyleNormal (no chroma).
	// Body cells start at x=2 (after '-' + ' ' prefix).
	for x := 2; x < 5; x++ {
		c := buf.Cell(x, 0)
		if c.Ch == 0 {
			continue
		}
		// Body should NOT have chroma-style FG (StyleNormal = empty Style).
		// We just check it's not the StyleDiffRemoved FG either.
		removedStyle := DefaultTheme{}.Style(StyleDiffRemoved)
		if c.St.FG == removedStyle.FG && c.St != (DefaultTheme{}.Style(StyleNormal)) {
			t.Errorf("'-' body cell at x=%d: should NOT have marker FG overlay (CC parity skip highlight); got %#v", x, c.St)
		}
	}
}

func TestDiff_T8_MarkerContextPrefixStyle(t *testing.T) {
	resetBlockCacheForTest()
	d := NewDiffFromBareLines(" foo")
	buf, _ := d.Render(ctxDiff(80))
	wantStyle := DefaultTheme{}.Style(StyleDiffContext)
	got := buf.Cell(0, 0).St
	if got != wantStyle {
		t.Errorf("' ' prefix style: want %#v (no marker color), got %#v", wantStyle, got)
	}
}

// ---- T1-9: hunk header style ----

func TestDiff_T9_HunkHeaderStyle(t *testing.T) {
	resetBlockCacheForTest()
	d := NewDiffFromHunks([]Hunk{{
		OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 1,
		Lines: []LineEntry{{Marker: '+', Text: "x"}},
	}})
	buf, _ := d.Render(ctxDiff(80))
	wantStyle := DefaultTheme{}.Style(StyleDiffHunkHeader)
	got := buf.Cell(0, 0).St
	if got != wantStyle {
		t.Errorf("hunk header style: want %#v, got %#v", wantStyle, got)
	}
}

// ---- T1-10..T1-12: gutter (R2 HIGH-4 ★A single-col CC parity) ----

func TestDiff_T10_GutterContextAdvance(t *testing.T) {
	resetBlockCacheForTest()
	// OldStart=10, NewStart=20, 3 context lines → gutter shows newNo
	d := NewDiffFromHunks([]Hunk{{
		OldStart: 10, OldLines: 3, NewStart: 20, NewLines: 3,
		Lines: []LineEntry{
			{Marker: ' ', Text: "a"},
			{Marker: ' ', Text: "b"},
			{Marker: ' ', Text: "c"},
		},
	}})
	buf, _ := d.Render(ctxDiff(80))
	// Row 1 (first body) should show gutter "20" (newNo for context).
	// Rows: 0=header, 1=line1, 2=line2, 3=line3.
	row1 := rowAsString(buf, 1)
	if !strings.HasPrefix(row1, "20 ") {
		t.Errorf("context line gutter: want '20 ', got %q", row1)
	}
}

func TestDiff_T11_GutterAddedOnlyNewNo(t *testing.T) {
	resetBlockCacheForTest()
	d := NewDiffFromHunks([]Hunk{{
		OldStart: 5, OldLines: 1, NewStart: 5, NewLines: 2,
		Lines: []LineEntry{
			{Marker: ' ', Text: "ctx"},
			{Marker: '+', Text: "added"},
		},
	}})
	buf, _ := d.Render(ctxDiff(80))
	row2 := rowAsString(buf, 2) // body row 2 = '+' added
	if !strings.HasPrefix(row2, "6") {
		t.Errorf("'+' line gutter: want newNo=6 prefix, got %q", row2)
	}
}

func TestDiff_T12_GutterRemovedOnlyOldNo(t *testing.T) {
	resetBlockCacheForTest()
	d := NewDiffFromHunks([]Hunk{{
		OldStart: 5, OldLines: 2, NewStart: 5, NewLines: 1,
		Lines: []LineEntry{
			{Marker: ' ', Text: "ctx"},
			{Marker: '-', Text: "removed"},
		},
	}})
	buf, _ := d.Render(ctxDiff(80))
	row2 := rowAsString(buf, 2) // body row 2 = '-' removed
	if !strings.HasPrefix(row2, "6") {
		t.Errorf("'-' line gutter: want oldNo=6 prefix, got %q", row2)
	}
}

// ---- T1-13..T1-17: body lang highlight ----

func TestDiff_T13_BodyLangGo(t *testing.T) {
	resetBlockCacheForTest()
	d := NewDiffFromBareLines("+func main() {}")
	d.BodyLang = "go"
	buf, _ := d.Render(ctxDiff(80))
	// At least one body cell should have non-zero FG (chroma highlight).
	hasHighlight := false
	for x := 2; x < 30; x++ {
		if buf.Cell(x, 0).St.FG != 0 {
			hasHighlight = true
			break
		}
	}
	if !hasHighlight {
		t.Errorf("body lang go on '+' line: want highlighted cells, got plain")
	}
}

func TestDiff_T14_BodyLangPython(t *testing.T) {
	resetBlockCacheForTest()
	d := NewDiffFromBareLines("+def x(): pass")
	d.BodyLang = "python"
	buf, _ := d.Render(ctxDiff(80))
	_, rows := buf.Size()
	if rows == 0 {
		t.Errorf("python body: want >0 rows")
	}
}

func TestDiff_T15_BodyLangEmpty(t *testing.T) {
	resetBlockCacheForTest()
	d := NewDiffFromBareLines("+plain text")
	buf, _ := d.Render(ctxDiff(80))
	_, rows := buf.Size()
	if rows == 0 {
		t.Errorf("empty body lang: want >0 rows")
	}
}

func TestDiff_T16_BodyLangUnknownFallback(t *testing.T) {
	resetBlockCacheForTest()
	d := NewDiffFromBareLines("+x")
	d.BodyLang = "not-a-lang-zzz"
	buf, _ := d.Render(ctxDiff(80))
	_, rows := buf.Size()
	if rows == 0 {
		t.Errorf("unknown lang: want >0 rows (fallback plain)")
	}
}

func TestDiff_T17_RemovedLineSkipHighlight_CCParity(t *testing.T) {
	resetBlockCacheForTest()
	// R3 CRIT-2 ★A regression: `-` line MUST NOT chroma-highlight body
	// even when BodyLang non-empty.
	d := NewDiffFromBareLines("-func main()")
	d.BodyLang = "go"
	buf, _ := d.Render(ctxDiff(80))
	// Body cells (after '- ' prefix at x=0,1) should be plain text.
	// chroma highlighted Go would set non-zero FG on keyword 'func' cells.
	// Body 'f' at x=2 should have empty FG.
	c := buf.Cell(2, 0)
	if c.Ch != 'f' {
		t.Fatalf("body cell 2: want 'f', got %q", c.Ch)
	}
	if c.St.FG != 0 {
		t.Errorf("'-' line body chroma skip (R3 CRIT-2 ★A): want zero FG, got FG=%#v (chroma should be skipped per CC color-diff:915-918)", c.St.FG)
	}
}

// ---- T1-18..T1-23: ParseUnified ----

func TestDiff_T18_ParseUnifiedValid(t *testing.T) {
	src := "--- a/x\n+++ b/x\n@@ -1,1 +1,1 @@\n-old\n+new\n"
	d, err := ParseUnified(src)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if d.FilePath != "x" {
		t.Errorf("FilePath: want 'x', got %q", d.FilePath)
	}
	if len(d.Hunks) != 1 || len(d.Hunks[0].Lines) != 2 {
		t.Errorf("hunks shape: %+v", d.Hunks)
	}
}

func TestDiff_T19_ParseUnifiedMalformedHeader(t *testing.T) {
	src := "@@ bad header @@\n+x\n"
	_, err := ParseUnified(src)
	if err == nil {
		t.Errorf("malformed header: want error, got nil")
	}
}

func TestDiff_T20_ParseUnifiedMissingBody(t *testing.T) {
	src := "--- a/x\n+++ b/x\n"
	d, err := ParseUnified(src)
	if err != nil {
		t.Errorf("only headers: want no error (empty hunks OK), got %v", err)
	}
	if len(d.Hunks) != 0 {
		t.Errorf("only headers: want 0 hunks, got %d", len(d.Hunks))
	}
}

func TestDiff_T21_ParseUnifiedGitExtrasStrip(t *testing.T) {
	src := "diff --git a/x b/x\nindex abc..def 100644\n--- a/x\n+++ b/x\n@@ -1,1 +1,1 @@\n-old\n+new\n"
	d, err := ParseUnified(src)
	if err != nil {
		t.Fatalf("git-extras strip: %v", err)
	}
	if len(d.Hunks) != 1 {
		t.Errorf("want 1 hunk after git-extras strip, got %d", len(d.Hunks))
	}
}

func TestDiff_T22_ParseUnifiedNoNewlineAtEOF(t *testing.T) {
	// HIGH-3: `\ No newline at end of file` NOT counted, NOT in Lines.
	src := "@@ -1,3 +1,3 @@\n line1\n line2\n-line3\n\\ No newline at end of file\n+line3new\n"
	d, err := ParseUnified(src)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if len(d.Hunks) != 1 {
		t.Fatalf("want 1 hunk, got %d", len(d.Hunks))
	}
	// Body should have 4 lines (line1, line2, -line3, +line3new) — NOT 5
	// (the `\` line discarded per HIGH-3).
	if got := len(d.Hunks[0].Lines); got != 4 {
		t.Errorf("Hunk.Lines: want 4 (no `\\` entry), got %d", got)
	}
}

func TestDiff_T23_ParseUnifiedTolerantLineCount(t *testing.T) {
	// Q7 ★B + R3 MED-2: tolerant — declared 3 but actual 2 → slog.Warn,
	// no error. Just verify no error returned.
	src := "@@ -1,3 +1,3 @@\n-a\n+b\n"
	_, err := ParseUnified(src)
	if err != nil {
		t.Errorf("count mismatch tolerant Q7 ★B: want no error, got %v", err)
	}
}

// ---- T1-24..T1-26: fence dispatch (spec-1.11 walker D-5) ----

func TestDiff_T24_FenceDispatchDiff(t *testing.T) {
	resetBlockCacheForTest()
	m := NewMarkdown("```diff\n+a\n-b\n```")
	buf, _ := m.Render(Context{Cols: 80, Wrap: WrapSoft, ColorDepth: 16777216})
	_, rows := buf.Size()
	if rows < 2 {
		t.Fatalf("fence diff: want ≥2 rows, got %d", rows)
	}
	// Row 0 cell 0 should be '+' (bare-lines mode no @@ header).
	if c := buf.Cell(0, 0).Ch; c != '+' {
		t.Errorf("fence diff row 0 cell 0: want '+', got %q", c)
	}
}

func TestDiff_T25_FenceDispatchPatch(t *testing.T) {
	resetBlockCacheForTest()
	m := NewMarkdown("```patch\n+x\n```")
	buf, _ := m.Render(Context{Cols: 80, Wrap: WrapSoft, ColorDepth: 16777216})
	_, rows := buf.Size()
	if rows < 1 {
		t.Errorf("fence patch alias: want ≥1 row, got %d", rows)
	}
}

func TestDiff_T26_FenceDispatchGoUnchanged(t *testing.T) {
	resetBlockCacheForTest()
	// spec-1.11 fence regression: non-diff lang unchanged path.
	m := NewMarkdown("```go\nfunc main() {}\n```")
	buf, _ := m.Render(Context{Cols: 80, Wrap: WrapSoft, ColorDepth: 16777216})
	_, rows := buf.Size()
	if rows < 2 {
		t.Errorf("fence go: want ≥2 rows (header+body via renderCodeBlock), got %d", rows)
	}
}

// ---- T1-27..T1-29: ColorDepth downgrade ----

func TestDiff_T27_ColorDepth16(t *testing.T) {
	resetBlockCacheForTest()
	d := NewDiffFromBareLines("+x")
	ctx := ctxDiff(80)
	ctx.ColorDepth = 16
	buf, _ := d.Render(ctx)
	_, rows := buf.Size()
	if rows == 0 {
		t.Errorf("ColorDepth=16: want >0 rows")
	}
}

func TestDiff_T28_ColorDepth256(t *testing.T) {
	resetBlockCacheForTest()
	d := NewDiffFromBareLines("+x")
	ctx := ctxDiff(80)
	ctx.ColorDepth = 256
	buf, _ := d.Render(ctx)
	_, rows := buf.Size()
	if rows == 0 {
		t.Errorf("ColorDepth=256: want >0 rows")
	}
}

func TestDiff_T29_ColorDepthTrueColor(t *testing.T) {
	resetBlockCacheForTest()
	d := NewDiffFromBareLines("+x")
	ctx := ctxDiff(80)
	ctx.ColorDepth = 16777216
	buf, _ := d.Render(ctx)
	_, rows := buf.Size()
	if rows == 0 {
		t.Errorf("ColorDepth=truecolor: want >0 rows")
	}
}

// ---- T1-30..T1-32: cache ----

func TestDiff_T30_CacheHitSameBuffer(t *testing.T) {
	resetBlockCacheForTest()
	d := NewDiffFromHunks([]Hunk{{
		OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 1,
		Lines: []LineEntry{{Marker: '+', Text: "x"}},
	}})
	buf1, _ := d.Render(ctxDiff(80))
	buf2, _ := d.Render(ctxDiff(80))
	if fmt.Sprintf("%p", buf1) != fmt.Sprintf("%p", buf2) {
		t.Errorf("cache hit (I-8): want same Buffer ref")
	}
}

func TestDiff_T31_CacheMissBodyLang(t *testing.T) {
	resetBlockCacheForTest()
	d1 := NewDiffFromBareLines("+x")
	d1.BodyLang = "go"
	d2 := NewDiffFromBareLines("+x")
	d2.BodyLang = "python"
	buf1, _ := d1.Render(ctxDiff(80))
	buf2, _ := d2.Render(ctxDiff(80))
	if fmt.Sprintf("%p", buf1) == fmt.Sprintf("%p", buf2) {
		t.Errorf("different BodyLang: want different cache entry")
	}
}

func TestDiff_T32_CacheMissFilePath(t *testing.T) {
	resetBlockCacheForTest()
	hunks := []Hunk{{
		OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 1,
		Lines: []LineEntry{{Marker: '+', Text: "x"}},
	}}
	d1 := Diff{Hunks: hunks, FilePath: "a.go"}
	d2 := Diff{Hunks: hunks, FilePath: "b.go"}
	buf1, _ := d1.Render(ctxDiff(80))
	buf2, _ := d2.Render(ctxDiff(80))
	if fmt.Sprintf("%p", buf1) == fmt.Sprintf("%p", buf2) {
		t.Errorf("different FilePath: want different cache entry")
	}
}

// ---- T1-33: concurrent race ----

func TestDiff_T33_ConcurrentRenderRace(t *testing.T) {
	resetBlockCacheForTest()
	d := NewDiffFromBareLines("+a\n-b\n c")
	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				_, _ = d.Render(ctxDiff(80))
			}
		}()
	}
	wg.Wait()
}

// ---- T1-34..T1-35: edge fast paths (LOW-2) ----

func TestDiff_T34_ColsZero(t *testing.T) {
	resetBlockCacheForTest()
	d := NewDiffFromBareLines("+x")
	buf, _ := d.Render(Context{Cols: 0})
	cols, rows := buf.Size()
	if cols != 0 || rows != 0 {
		t.Errorf("Cols=0 fast path: want (0,0), got (%d,%d)", cols, rows)
	}
}

func TestDiff_T35_MeasureOnlyAccuracy(t *testing.T) {
	resetBlockCacheForTest()
	d := NewDiffFromHunks([]Hunk{{
		OldStart: 1, OldLines: 2, NewStart: 1, NewLines: 2,
		Lines: []LineEntry{
			{Marker: '+', Text: "a"},
			{Marker: '-', Text: "b"},
		},
	}})
	ctx := ctxDiff(80)
	ctx.MeasureOnly = true
	buf, _ := d.Render(ctx)
	_, rows := buf.Size()
	if rows != 3 { // header + 2 body
		t.Errorf("MeasureOnly: want 3 rows, got %d", rows)
	}
	// Cell content should be zero (no writes).
	if c := buf.Cell(0, 0); c.Ch != 0 {
		t.Errorf("MeasureOnly: want zero cell, got %q", c.Ch)
	}
}

// ---- errcode sanity ----

func TestDiff_ErrDiffParseHunkHeader_Sentinel(t *testing.T) {
	_, err := ParseUnified("@@ malformed @@\n")
	if err == nil {
		t.Fatalf("want error")
	}
	// R2 HIGH-1 fix: verify the error actually wraps the registered
	// sentinel (the previous `errors.Is(err, err)` was a tautology that
	// passed for any non-nil error and did NOT verify the contract).
	if !errors.Is(err, ErrDiffParseHunkHeader) {
		t.Errorf("errors.Is(err, ErrDiffParseHunkHeader): want true, got false; err=%v", err)
	}
}

// ---- R2 MED-2: NewDiff auto-detect coverage ----

func TestDiff_NewDiff_UnifiedSuccess(t *testing.T) {
	src := "--- a/old.go\n+++ b/new.go\n@@ -1,2 +1,2 @@\n-old\n+new\n"
	d := NewDiff(src, "explicit.go")
	if len(d.Hunks) != 1 {
		t.Fatalf("want 1 hunk, got %d", len(d.Hunks))
	}
	// Caller-supplied filePath overrides parser-extracted +++ header.
	if d.FilePath != "explicit.go" {
		t.Errorf("FilePath: want explicit.go, got %q", d.FilePath)
	}
	if d.Hunks[0].Lines[0].Marker != '-' || d.Hunks[0].Lines[1].Marker != '+' {
		t.Errorf("marker classification dropped: %v", d.Hunks[0].Lines)
	}
}

func TestDiff_NewDiff_FallbackToBareLines(t *testing.T) {
	// Malformed `@@` header → ParseUnified errors → NewDiff falls back
	// to bare-lines with filePath preserved.
	src := "@@ totally malformed @@\n+added\n-removed\n"
	d := NewDiff(src, "fallback.go")
	if d.FilePath != "fallback.go" {
		t.Errorf("FilePath preserved on fallback: got %q", d.FilePath)
	}
	if len(d.Hunks) != 1 {
		t.Fatalf("want 1 bare-lines hunk, got %d", len(d.Hunks))
	}
	if d.Hunks[0].OldStart != 0 {
		t.Errorf("bare-lines hunk OldStart: want 0, got %d", d.Hunks[0].OldStart)
	}
	// bare-lines parser swallows the @@ line (treated as non-marker '?').
	markers := make([]rune, 0, len(d.Hunks[0].Lines))
	for _, ln := range d.Hunks[0].Lines {
		markers = append(markers, ln.Marker)
	}
	// Want at least one '+' and one '-' from the body.
	gotPlus, gotMinus := false, false
	for _, m := range markers {
		if m == '+' {
			gotPlus = true
		}
		if m == '-' {
			gotMinus = true
		}
	}
	if !gotPlus || !gotMinus {
		t.Errorf("bare-lines fallback did not preserve +/- markers: %v", markers)
	}
}

// rowAsString reconstructs a row's text content from grid cells.
func rowAsString(buf interface {
	Cell(x, y int) buffer.Cell
	Size() (int, int)
}, y int) string {
	cols, _ := buf.Size()
	var b strings.Builder
	for x := 0; x < cols; x++ {
		c := buf.Cell(x, y)
		if c.Ch == 0 {
			b.WriteByte(' ')
			continue
		}
		b.WriteRune(c.Ch)
	}
	return strings.TrimRight(b.String(), " ")
}
