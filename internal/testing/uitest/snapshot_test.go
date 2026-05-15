// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

//go:build !windows

package uitest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSnapshotGolden_RoundTrip exercises the SnapshotGolden public API
// end-to-end: launch helper child → wait for output → snapshot → compare
// to a golden written under the *caller's* testdata dir (not the uitest
// package dir). T-13 codex HIGH-2 + claude MED-1.
func TestSnapshotGolden_RoundTrip(t *testing.T) {
	// Sets up a golden under this test file's testdata/golden/TestName.golden
	// by directly seeding the file once, then asserting SnapshotGolden
	// finds + matches it.
	wantDir := filepath.Join("testdata", "golden", t.Name())
	if err := os.MkdirAll(wantDir, 0o750); err != nil {
		t.Fatalf("setup: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join("testdata", "golden", t.Name())) })

	term := Term(t, helperCmd(t, "banner"), 80, 5)
	term.WaitFor(t, func(*Terminal) bool {
		return strings.Contains(strings.Join(term.CellGrid(), "\n"), "ready")
	}, time.Second)

	// Write the current CellGrid as the golden, then re-call SnapshotGolden
	// to verify it matches.
	got := strings.Join(term.CellGrid(), "\n")
	goldPath := filepath.Join(wantDir, "snap.golden")
	if err := os.WriteFile(goldPath, []byte(got), 0o600); err != nil {
		t.Fatalf("write golden: %v", err)
	}
	term.SnapshotGolden(t, "snap")
}
