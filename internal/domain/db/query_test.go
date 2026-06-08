// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package db

import "testing"

// TestQueryOptions_EffectiveDefaults — zero fields fall back to the
// Default* constants; positive values pass through (spec-2.3a D-1).
func TestQueryOptions_EffectiveDefaults(t *testing.T) {
	t.Parallel()
	var zero QueryOptions
	if got := zero.EffectiveMaxRows(); got != DefaultMaxRows {
		t.Errorf("zero MaxRows = %d; want default %d", got, DefaultMaxRows)
	}
	if got := zero.EffectiveMaxCellRunes(); got != DefaultMaxCellRunes {
		t.Errorf("zero MaxCellRunes = %d; want default %d", got, DefaultMaxCellRunes)
	}
	custom := QueryOptions{MaxRows: 5, MaxCellRunes: 10}
	if custom.EffectiveMaxRows() != 5 || custom.EffectiveMaxCellRunes() != 10 {
		t.Errorf("custom passthrough failed: %+v", custom)
	}
	// Negative is treated as unset (defensive).
	neg := QueryOptions{MaxRows: -1, MaxCellRunes: -1}
	if neg.EffectiveMaxRows() != DefaultMaxRows || neg.EffectiveMaxCellRunes() != DefaultMaxCellRunes {
		t.Error("negative caps should fall back to defaults")
	}
}

// TestQueryResult_ZeroValue — a zero QueryResult is a usable empty page
// (no panic on len; both truncation flags false).
func TestQueryResult_ZeroValue(t *testing.T) {
	t.Parallel()
	var r QueryResult
	if len(r.Columns) != 0 || len(r.Rows) != 0 || r.RowsTruncated || r.CellsTruncated {
		t.Errorf("zero QueryResult not empty: %+v", r)
	}
}
