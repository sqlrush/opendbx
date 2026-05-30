// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File toolresult_cached_test.go — spec-1.22 D-6: the render-only "(cached)"
// dedup marker on block.ToolResult.

package block

import (
	"strings"
	"testing"
)

// TestToolResult_Cached_RendersMarker — a cached success renders its content
// row plus a trailing dim "(cached)" marker row.
func TestToolResult_Cached_RendersMarker(t *testing.T) {
	t.Parallel()
	tr := NewToolResult("idc", "Bash", "line1", false)
	tr.Cached = true
	buf, _ := tr.Render(ctxToolResult(80))
	_, rows := buf.Size()
	if rows != 2 {
		t.Fatalf("cached success: want 2 rows (content + cached marker), got %d", rows)
	}
	marker := resultRowText(t, buf, func(x, y int) rune { return buf.Cell(x, y).Ch }, 1)
	if !strings.Contains(marker, "(cached)") {
		t.Errorf("cached marker row missing: %q", marker)
	}
}

// TestToolResult_Cached_EmptySuccess_MarkerStillVisible — codex MED-2: an
// empty-success hit whose content renders zero rows must STILL show the cached
// marker (it cannot silently disappear).
func TestToolResult_Cached_EmptySuccess_MarkerStillVisible(t *testing.T) {
	t.Parallel()
	// Same zero-row case as TestToolResult_Success_NoAdapter_ZeroRows.
	tr := NewToolResult("idce", "definitely_unknown_tool_xyz", "", false)
	tr.Cached = true
	buf, _ := tr.Render(ctxToolResult(80))
	_, rows := buf.Size()
	if rows != 1 {
		t.Fatalf("cached empty-success: want 1 row (cached marker), got %d", rows)
	}
	row0 := resultRowText(t, buf, func(x, y int) rune { return buf.Cell(x, y).Ch }, 0)
	if !strings.Contains(row0, "(cached)") {
		t.Errorf("cached marker missing for empty-success: %q", row0)
	}
}

// TestToolResult_NotCached_NoMarker — a non-cached result has no marker row.
func TestToolResult_NotCached_NoMarker(t *testing.T) {
	t.Parallel()
	tr := NewToolResult("idn", "Bash", "line1", false) // Cached defaults false
	buf, _ := tr.Render(ctxToolResult(80))
	_, rows := buf.Size()
	if rows != 1 {
		t.Fatalf("non-cached success: want 1 row, got %d", rows)
	}
	row0 := resultRowText(t, buf, func(x, y int) rune { return buf.Cell(x, y).Ch }, 0)
	if strings.Contains(row0, "(cached)") {
		t.Errorf("non-cached result must not have a marker: %q", row0)
	}
}
