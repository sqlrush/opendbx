// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

//go:build !windows

package block_test

import (
	"os"
	"strings"
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/block"
	"github.com/sqlrush/opendbx/internal/testing/uiinvariant"
	"github.com/sqlrush/opendbx/internal/testing/visualgolden"
)

// TestCompactVisualGolden consumes 4 CompactSummary CC fixtures parked
// under tests/integration/uitest/block/testdata/visual/Compact*/.
//
// Fixture coverage (spec-1.10 D-8 § 4.2 INT-3):
//
//	CompactReadOnly       — Read-only group (3 files), no hint
//	CompactSearchOnly     — Search-only group (2 patterns)
//	CompactMixed          — Search + Read + List + memory ops
//	CompactWithLongPaths  — Active group with long hint path (`..` guard)
//
// COMPACT_VISUAL_REQUIRED=1 turns missing fixtures into fatal (CI
// strict); without it tests t.Skip (developer-local convenience).
// Independent env gate per spec-1.10 D-8 lesson (decoupled from
// BLOCK_VISUAL_REQUIRED + TOOLUSE_VISUAL_REQUIRED + TOOLRESULT_VISUAL_REQUIRED).
func TestCompactVisualGolden(t *testing.T) {
	cases := []struct {
		name string
		c    block.CompactSummary
		cols int
	}{
		{
			name: "CompactReadOnly",
			c:    block.CompactSummary{ReadCount: 3},
			cols: 80,
		},
		{
			name: "CompactSearchOnly",
			c:    block.CompactSummary{SearchCount: 2},
			cols: 80,
		},
		{
			name: "CompactMixed",
			c: block.CompactSummary{
				SearchCount:      1,
				ReadCount:        2,
				ListCount:        3,
				MemoryReadCount:  1,
				MemoryWriteCount: 2,
			},
			cols: 80,
		},
		{
			name: "CompactWithLongPaths",
			c: block.CompactSummary{
				ReadCount:         5,
				IsActive:          true,
				LatestDisplayHint: "/tmp/some/deeply/nested/dir/important-file.go",
			},
			cols: 80,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx := ctxDefault(tc.cols)
			buf, err := tc.c.Render(ctx)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			raw := bufferANSI(t, buf)
			uiinvariant.CheckANSI(t, raw)
			png := visualgolden.Render(t, raw, visualgolden.DefaultTheme())
			fixturePath := visualFixturePath(t, tc.name, "golden.png")
			if !visualgolden.Update() && !visualFixtureExists(t, fixturePath) {
				// spec-1.10 D-8: independent COMPACT_VISUAL_REQUIRED env
				// gate (decoupled from peer gates for spec-1.7/1.9/1.9b).
				if os.Getenv("COMPACT_VISUAL_REQUIRED") != "" {
					t.Fatalf("missing CC visual fixture for %s (run capture SOP first)", tc.name)
				}
				t.Skipf("missing CC visual fixture for %s; spec-1.10 capture pending (set COMPACT_VISUAL_REQUIRED=1 once captured)", tc.name)
			}
			visualgolden.CompareFile(t, fixturePath, png, 0.02)
		})
	}
}

// TestCompactVisualGolden_ParkedFixtures verifies the 4 fixture parking
// dirs exist before harness runs.
func TestCompactVisualGolden_ParkedFixtures(t *testing.T) {
	t.Parallel()
	required := []string{
		"CompactReadOnly",
		"CompactSearchOnly",
		"CompactMixed",
		"CompactWithLongPaths",
	}
	for _, name := range required {
		path := visualFixturePath(t, name, "")
		path = strings.TrimSuffix(path, "/")
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			t.Errorf("parked fixture dir missing: %s (run spec-1.10 capture SOP)", path)
		}
	}
}
