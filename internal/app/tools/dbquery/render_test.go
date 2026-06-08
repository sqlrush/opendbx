// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package dbquery

import (
	"strings"
	"testing"

	"github.com/sqlrush/opendbx/internal/domain/db"
)

// TestRenderTable_Aligned — header + separator + rows + footer, columns
// left-aligned to max width, two-space gap, no trailing pad (spec-2.3a Q5).
func TestRenderTable_Aligned(t *testing.T) {
	t.Parallel()
	res := db.QueryResult{
		Columns: []string{"id", "name"},
		Rows:    [][]string{{"1", "alice"}, {"20", "bo"}},
	}
	got := renderTable(res)
	want := "id  name\n" +
		"--  -----\n" +
		"1   alice\n" +
		"20  bo\n" +
		"(2 rows)"
	if got != want {
		t.Errorf("renderTable =\n%q\nwant\n%q", got, want)
	}
}

// TestRenderTable_Empty — zero columns → "(0 rows)".
func TestRenderTable_Empty(t *testing.T) {
	t.Parallel()
	if got := renderTable(db.QueryResult{}); got != "(0 rows)" {
		t.Errorf("empty = %q; want (0 rows)", got)
	}
}

// TestRenderTable_TruncationFooters — rows/cells truncation annotations.
func TestRenderTable_TruncationFooters(t *testing.T) {
	t.Parallel()
	res := db.QueryResult{
		Columns:        []string{"c"},
		Rows:           [][]string{{"x"}},
		RowsTruncated:  true,
		CellsTruncated: true,
	}
	got := renderTable(res)
	if !strings.Contains(got, "(first 1 rows; result truncated) (some cells truncated)") {
		t.Errorf("footer missing truncation notes:\n%s", got)
	}
}

// TestCapUTF8_RuneSafe — a 16KiB cut never splits a multibyte rune
// (spec-2.3a codex MED-1; the 1.20.2 CJK mojibake class).
func TestCapUTF8_RuneSafe(t *testing.T) {
	t.Parallel()
	// Build a string well over the cap of all CJK runes (3 bytes each).
	big := strings.Repeat("世", maxOutputBytes) // 3*maxOutputBytes bytes
	got := capUTF8(big, maxOutputBytes)
	if len(got) > maxOutputBytes {
		t.Errorf("capUTF8 len %d > max %d", len(got), maxOutputBytes)
	}
	if !strings.HasSuffix(got, truncSuffix) {
		t.Error("missing truncation suffix")
	}
	// The body (minus suffix) must be valid UTF-8 (no split rune).
	body := strings.TrimSuffix(got, truncSuffix)
	if strings.ContainsRune(body, '�') {
		t.Error("replacement char — a rune was split")
	}
	for _, r := range body {
		if r != '世' {
			t.Errorf("unexpected rune %q — boundary split", r)
			break
		}
	}
}

// TestCapUTF8_UnderCap — short output passes through unchanged.
func TestCapUTF8_UnderCap(t *testing.T) {
	t.Parallel()
	s := "small table\n(1 rows)"
	if got := capUTF8(s, maxOutputBytes); got != s {
		t.Errorf("under-cap mutated: %q", got)
	}
}
