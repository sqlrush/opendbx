// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File compact_test.go — spec-1.10 D-7 unit test suite for CompactSummary
// production (≥ 28 case per R2/R4 matrix). Coverage gate ≥ 85% per
// CLAUDE rule 8.

package block

import (
	"strings"
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
)

func ctxCompact(cols int) Context {
	return Context{Cols: cols, Rows: 24, Wrap: WrapSoft}
}

// mustRenderCompact surfaces Render errors as fatal — R5 M-3 absorb
// (claude path 1/3 caught silent `_` discards across all 28 cases).
// CompactSummary.Render currently never returns non-nil error, but
// CLAUDE rule 7 requires explicit handling so regressions are caught.
func mustRenderCompact(t *testing.T, c CompactSummary, ctx Context) buffer.Buffer {
	t.Helper()
	buf, err := c.Render(ctx)
	if err != nil {
		t.Fatalf("CompactSummary.Render: unexpected error: %v", err)
	}
	return buf
}

func readCompactRow(t *testing.T, buf interface {
	Size() (int, int)
}, cellFn func(x, y int) rune, y int) string {
	t.Helper()
	cols, _ := buf.Size()
	var b strings.Builder
	for x := 0; x < cols; x++ {
		c := cellFn(x, y)
		if c == 0 {
			continue
		}
		b.WriteRune(c)
	}
	return b.String()
}

// === Dispatch gate (T1-1..4)

func TestCompact_Dispatch_DefaultCollapsed(t *testing.T) {
	t.Parallel()
	c := NewCompactSummary()
	c.ReadCount = 3
	buf := mustRenderCompact(t, c, ctxCompact(80))
	_, rows := buf.Size()
	if rows != 1 {
		t.Fatalf("default (IsTranscript=false, Verbose=false) collapsed: want 1 row, got %d", rows)
	}
}

func TestCompact_Dispatch_TranscriptExpanded(t *testing.T) {
	t.Parallel()
	c := NewCompactSummary()
	c.ReadCount = 3
	ctx := ctxCompact(80)
	ctx.IsTranscript = true
	buf := mustRenderCompact(t, c, ctx)
	_, rows := buf.Size()
	if rows != 0 {
		t.Fatalf("IsTranscript=true expanded: want 0 rows, got %d", rows)
	}
}

func TestCompact_Dispatch_VerboseExpanded(t *testing.T) {
	t.Parallel()
	c := NewCompactSummary()
	c.ReadCount = 3
	ctx := ctxCompact(80)
	ctx.Verbose = true
	buf := mustRenderCompact(t, c, ctx)
	_, rows := buf.Size()
	if rows != 0 {
		t.Fatalf("Verbose=true expanded: want 0 rows, got %d", rows)
	}
}

func TestCompact_Dispatch_BothExpanded(t *testing.T) {
	t.Parallel()
	c := NewCompactSummary()
	c.ReadCount = 3
	ctx := ctxCompact(80)
	ctx.IsTranscript = true
	ctx.Verbose = true
	buf := mustRenderCompact(t, c, ctx)
	_, rows := buf.Size()
	if rows != 0 {
		t.Fatalf("both true expanded: want 0 rows, got %d", rows)
	}
}

// === Single category plural (T1-5..10)

func TestCompact_ReadCount_Singular(t *testing.T) {
	t.Parallel()
	c := NewCompactSummary()
	c.ReadCount = 1
	buf := mustRenderCompact(t, c, ctxCompact(80))
	row0 := readCompactRow(t, buf, func(x, y int) rune { return buf.Cell(x, y).Ch }, 0)
	if !strings.Contains(row0, "Read 1 file") || strings.Contains(row0, "files") {
		t.Errorf("singular: want 'Read 1 file' (no 's'), got %q", row0)
	}
}

func TestCompact_ReadCount_Plural(t *testing.T) {
	t.Parallel()
	c := NewCompactSummary()
	c.ReadCount = 3
	buf := mustRenderCompact(t, c, ctxCompact(80))
	row0 := readCompactRow(t, buf, func(x, y int) rune { return buf.Cell(x, y).Ch }, 0)
	if !strings.Contains(row0, "Read 3 files") {
		t.Errorf("plural: want 'Read 3 files', got %q", row0)
	}
}

func TestCompact_ListCount_DirectorySingular(t *testing.T) {
	t.Parallel()
	c := NewCompactSummary()
	c.ListCount = 1
	buf := mustRenderCompact(t, c, ctxCompact(80))
	row0 := readCompactRow(t, buf, func(x, y int) rune { return buf.Cell(x, y).Ch }, 0)
	if !strings.Contains(row0, "1 directory") || strings.Contains(row0, "directories") {
		t.Errorf("singular: want '1 directory' (no 'ies'), got %q", row0)
	}
}

func TestCompact_ListCount_DirectoriesPlural(t *testing.T) {
	t.Parallel()
	c := NewCompactSummary()
	c.ListCount = 5
	buf := mustRenderCompact(t, c, ctxCompact(80))
	row0 := readCompactRow(t, buf, func(x, y int) rune { return buf.Cell(x, y).Ch }, 0)
	if !strings.Contains(row0, "5 directories") {
		t.Errorf("plural: want '5 directories', got %q", row0)
	}
	// Critical: must NOT use "dirs" (R2 CRIT-2)
	if strings.Contains(row0, "dirs") {
		t.Errorf("must NOT use 'dirs' (R2 CRIT-2), got %q", row0)
	}
}

func TestCompact_SearchCount_PatternsPlural(t *testing.T) {
	t.Parallel()
	c := NewCompactSummary()
	c.SearchCount = 2
	buf := mustRenderCompact(t, c, ctxCompact(80))
	row0 := readCompactRow(t, buf, func(x, y int) rune { return buf.Cell(x, y).Ch }, 0)
	if !strings.Contains(row0, "2 patterns") {
		t.Errorf("want '2 patterns', got %q", row0)
	}
}

func TestCompact_SearchCount_PatternSingular(t *testing.T) {
	t.Parallel()
	c := NewCompactSummary()
	c.SearchCount = 1
	buf := mustRenderCompact(t, c, ctxCompact(80))
	row0 := readCompactRow(t, buf, func(x, y int) rune { return buf.Cell(x, y).Ch }, 0)
	if !strings.Contains(row0, "1 pattern") || strings.Contains(row0, "patterns") {
		t.Errorf("singular: want '1 pattern' (no 's'), got %q", row0)
	}
}

// === Mixed + separator (T1-11..14)

func TestCompact_Mixed_ReadSearch(t *testing.T) {
	t.Parallel()
	c := NewCompactSummary()
	c.ReadCount = 2
	c.SearchCount = 3
	buf := mustRenderCompact(t, c, ctxCompact(80))
	row0 := readCompactRow(t, buf, func(x, y int) rune { return buf.Cell(x, y).Ch }, 0)
	// Search comes first (non-mem order: search/read/list).
	if !strings.Contains(row0, "Searched for 3 patterns") {
		t.Errorf("want 'Searched for 3 patterns' first, got %q", row0)
	}
	if !strings.Contains(row0, "read 2 files") {
		t.Errorf("want 'read 2 files' (lowercase non-first), got %q", row0)
	}
	// Separator must be ", " (R2 CRIT-2)
	if !strings.Contains(row0, ", ") {
		t.Errorf("want comma-space separator, got %q", row0)
	}
	if strings.Contains(row0, "·") {
		t.Errorf("must NOT use '·' separator (R2 CRIT-2), got %q", row0)
	}
}

func TestCompact_Mixed_ReadList(t *testing.T) {
	t.Parallel()
	c := NewCompactSummary()
	c.ReadCount = 5
	c.ListCount = 2
	buf := mustRenderCompact(t, c, ctxCompact(80))
	row0 := readCompactRow(t, buf, func(x, y int) rune { return buf.Cell(x, y).Ch }, 0)
	if !strings.Contains(row0, "Read 5 files") || !strings.Contains(row0, "listed 2 directories") {
		t.Errorf("mixed: %q", row0)
	}
}

func TestCompact_Mixed_AllThree(t *testing.T) {
	t.Parallel()
	c := NewCompactSummary()
	c.SearchCount = 1
	c.ReadCount = 2
	c.ListCount = 3
	buf := mustRenderCompact(t, c, ctxCompact(120))
	row0 := readCompactRow(t, buf, func(x, y int) rune { return buf.Cell(x, y).Ch }, 0)
	// CC order: Search, Read, List
	if !strings.Contains(row0, "Searched for 1 pattern, read 2 files, listed 3 directories") {
		t.Errorf("expected CC order: %q", row0)
	}
}

func TestCompact_Mixed_SeparatorOnlyComma(t *testing.T) {
	t.Parallel()
	c := NewCompactSummary()
	c.ReadCount = 1
	c.SearchCount = 1
	buf := mustRenderCompact(t, c, ctxCompact(80))
	row0 := readCompactRow(t, buf, func(x, y int) rune { return buf.Cell(x, y).Ch }, 0)
	if strings.Contains(row0, "·") {
		t.Errorf("must NOT use '·': %q", row0)
	}
}

// === Memory ops (T1-15..18; R4 MED-3 nonMem-first + per-op + lowercase non-first)

func TestCompact_Memory_NonMemoryFirst(t *testing.T) {
	t.Parallel()
	c := NewCompactSummary()
	c.ReadCount = 2
	c.MemoryReadCount = 1
	buf := mustRenderCompact(t, c, ctxCompact(120))
	row0 := readCompactRow(t, buf, func(x, y int) rune { return buf.Cell(x, y).Ch }, 0)
	// non-memory part (Read) first, memory part after (per R4 MED-3 visible order).
	readIdx := strings.Index(row0, "Read 2 files")
	recalledIdx := strings.Index(row0, "recalled 1 memory")
	if readIdx < 0 || recalledIdx < 0 {
		t.Fatalf("missing parts: %q", row0)
	}
	if readIdx > recalledIdx {
		t.Errorf("non-memory must come BEFORE memory (R4 MED-3), got %q", row0)
	}
}

func TestCompact_Memory_RecalledPerOp(t *testing.T) {
	t.Parallel()
	c := NewCompactSummary()
	c.MemoryReadCount = 3
	buf := mustRenderCompact(t, c, ctxCompact(80))
	row0 := readCompactRow(t, buf, func(x, y int) rune { return buf.Cell(x, y).Ch }, 0)
	if !strings.Contains(row0, "Recalled 3 memories") {
		t.Errorf("want 'Recalled 3 memories' (per-op pattern), got %q", row0)
	}
	if strings.Contains(row0, "Memory:") {
		t.Errorf("must NOT use 'Memory: …' suffix (R2 HIGH-1), got %q", row0)
	}
}

func TestCompact_Memory_SearchedNoCountWord(t *testing.T) {
	t.Parallel()
	c := NewCompactSummary()
	c.MemorySearchCount = 1
	buf := mustRenderCompact(t, c, ctxCompact(80))
	row0 := readCompactRow(t, buf, func(x, y int) rune { return buf.Cell(x, y).Ch }, 0)
	// CC pattern: "Searched memories" without count word.
	if !strings.Contains(row0, "Searched memories") {
		t.Errorf("want 'Searched memories', got %q", row0)
	}
}

func TestCompact_Memory_WroteWithCount(t *testing.T) {
	t.Parallel()
	c := NewCompactSummary()
	c.MemoryWriteCount = 2
	buf := mustRenderCompact(t, c, ctxCompact(80))
	row0 := readCompactRow(t, buf, func(x, y int) rune { return buf.Cell(x, y).Ch }, 0)
	if !strings.Contains(row0, "Wrote 2 memories") {
		t.Errorf("want 'Wrote 2 memories', got %q", row0)
	}
}

// === Tense (active vs past)

func TestCompact_Tense_Active_PresentVerb(t *testing.T) {
	t.Parallel()
	c := NewCompactSummary()
	c.ReadCount = 2
	c.IsActive = true
	buf := mustRenderCompact(t, c, ctxCompact(80))
	row0 := readCompactRow(t, buf, func(x, y int) rune { return buf.Cell(x, y).Ch }, 0)
	if !strings.Contains(row0, "Reading 2 files") {
		t.Errorf("active → present 'Reading', got %q", row0)
	}
}

func TestCompact_Tense_Inactive_PastVerb(t *testing.T) {
	t.Parallel()
	c := NewCompactSummary()
	c.ReadCount = 2
	c.IsActive = false
	buf := mustRenderCompact(t, c, ctxCompact(80))
	row0 := readCompactRow(t, buf, func(x, y int) rune { return buf.Cell(x, y).Ch }, 0)
	if !strings.Contains(row0, "Read 2 files") {
		t.Errorf("inactive → past 'Read', got %q", row0)
	}
}

// === Hint row (T1-19..22)

func TestCompact_HintRow_ActiveWithHint(t *testing.T) {
	t.Parallel()
	c := NewCompactSummary()
	c.ReadCount = 1
	c.IsActive = true
	c.LatestDisplayHint = "/etc/hosts"
	buf := mustRenderCompact(t, c, ctxCompact(80))
	_, rows := buf.Size()
	if rows < 2 {
		t.Fatalf("active + hint: want ≥2 rows, got %d", rows)
	}
	row1 := readCompactRow(t, buf, func(x, y int) rune { return buf.Cell(x, y).Ch }, 1)
	if !strings.Contains(row1, "hosts") {
		t.Errorf("hint row missing path basename, got %q", row1)
	}
}

func TestCompact_HintRow_InactiveNoHint(t *testing.T) {
	t.Parallel()
	c := NewCompactSummary()
	c.ReadCount = 1
	c.IsActive = false
	c.LatestDisplayHint = "/etc/hosts"
	buf := mustRenderCompact(t, c, ctxCompact(80))
	_, rows := buf.Size()
	if rows != 1 {
		t.Fatalf("inactive: hint row must NOT show (per CC B-26), got %d rows", rows)
	}
}

func TestCompact_HintRow_EmptyHintNoRow(t *testing.T) {
	t.Parallel()
	c := NewCompactSummary()
	c.ReadCount = 1
	c.IsActive = true
	c.LatestDisplayHint = ""
	buf := mustRenderCompact(t, c, ctxCompact(80))
	_, rows := buf.Size()
	if rows != 1 {
		t.Fatalf("empty hint: want 1 row, got %d", rows)
	}
}

func TestCompact_HintRow_CJKPath(t *testing.T) {
	t.Parallel()
	c := NewCompactSummary()
	c.ReadCount = 1
	c.IsActive = true
	c.LatestDisplayHint = "/tmp/中文测试.txt"
	buf := mustRenderCompact(t, c, ctxCompact(80))
	row1 := readCompactRow(t, buf, func(x, y int) rune { return buf.Cell(x, y).Ch }, 1)
	if !strings.ContainsRune(row1, '中') {
		t.Errorf("CJK path lost: %q", row1)
	}
}

// === Hint never appends elapsed (R4 MED-4)

func TestCompact_HintRow_NoElapsedSuffix(t *testing.T) {
	t.Parallel()
	c := NewCompactSummary()
	c.ReadCount = 1
	c.IsActive = true
	c.LatestDisplayHint = "/tmp/x"
	buf := mustRenderCompact(t, c, ctxCompact(80))
	row1 := readCompactRow(t, buf, func(x, y int) rune { return buf.Cell(x, y).Ch }, 1)
	// hint row must NOT contain "s)" suffix or "ms" duration (R4 MED-4 / ❌-7).
	if strings.Contains(row1, "ms") || strings.Contains(row1, "s)") {
		t.Errorf("hint row must NOT append elapsed (R4 MED-4), got %q", row1)
	}
}

// === Edge cases (T1-25..27)

func TestCompact_AllZero_NoRows(t *testing.T) {
	t.Parallel()
	c := NewCompactSummary()
	buf := mustRenderCompact(t, c, ctxCompact(80))
	_, rows := buf.Size()
	if rows != 0 {
		t.Fatalf("all zero degenerate: want 0 rows, got %d", rows)
	}
}

func TestCompact_ZeroCols(t *testing.T) {
	t.Parallel()
	c := NewCompactSummary()
	c.ReadCount = 1
	buf := mustRenderCompact(t, c, Context{Cols: 0})
	cols, rows := buf.Size()
	if cols != 0 || rows != 0 {
		t.Errorf("ctx.Cols=0: want (0,0), got (%d,%d)", cols, rows)
	}
}

func TestCompact_MeasureOnly(t *testing.T) {
	t.Parallel()
	c := NewCompactSummary()
	c.ReadCount = 3
	c.IsActive = true
	c.LatestDisplayHint = "/tmp/x"
	ctx := Context{Cols: 80, Rows: 24, MeasureOnly: true}
	buf := mustRenderCompact(t, c, ctx)
	_, rows := buf.Size()
	if rows != 2 {
		t.Fatalf("MeasureOnly: want 2 rows (summary + hint), got %d", rows)
	}
}

// === compactIndicator placeholder sanity (T1-25; R2.1.3 MED-2)

func TestCompactIndicator_PlaceholderSanity(t *testing.T) {
	t.Parallel()
	ind := compactIndicator(ToolGroupCategoryDefault)
	if ind.Rune == 0 {
		t.Errorf("compactIndicator: empty placeholder rune")
	}
}

// === expandBlockRows regression (T1-28)

func TestExpandBlockRows_NoSplitNewlines(t *testing.T) {
	t.Parallel()
	rows := []blockRow{{text: "abc\ndef", style: StyleNormal}}
	out := expandBlockRows(Context{Cols: 80, Wrap: WrapSoft}, rows, ExpandOptions{SplitNewlines: false})
	// Without split, wrap() may or may not split on \n itself; just verify
	// no panic + at least 1 row output.
	if len(out) < 1 {
		t.Errorf("no-split: want ≥1 row, got %d", len(out))
	}
}

func TestExpandBlockRows_SplitNewlines(t *testing.T) {
	t.Parallel()
	rows := []blockRow{{text: "abc\ndef", style: StyleNormal}}
	out := expandBlockRows(Context{Cols: 80, Wrap: WrapSoft}, rows, ExpandOptions{SplitNewlines: true})
	if len(out) != 2 {
		t.Errorf("split: want 2 rows (abc / def), got %d", len(out))
	}
	if out[0].text != "abc" || out[1].text != "def" {
		t.Errorf("split rows: got %v", out)
	}
}

func TestExpandBlockRows_EmptyRowPreserved(t *testing.T) {
	t.Parallel()
	rows := []blockRow{{text: "", style: StyleNormal}}
	out := expandBlockRows(Context{Cols: 80, Wrap: WrapSoft}, rows, ExpandOptions{SplitNewlines: false})
	if len(out) != 1 {
		t.Errorf("empty row preserve: want 1 row, got %d", len(out))
	}
}
