// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

//go:build !windows

package uitest

import (
	"strings"
	"testing"

	"github.com/sqlrush/opendbx/internal/testing/golden"
)

// SnapshotGolden compares the current cell grid (rows joined by '\n')
// against testdata/golden/<TestName>[/<name>].golden via the project
// internal/testing/golden package. Fatal on mismatch.
//
// Pass -update to refresh the golden file.
func (term *Terminal) SnapshotGolden(t testing.TB, name string) {
	t.Helper()
	got := strings.Join(term.CellGrid(), "\n")
	golden.Compare(t, name, []byte(got))
}
