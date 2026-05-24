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

// TestDiffVisualGolden consumes 5 Diff CC fixtures parked under
// tests/integration/uitest/block/testdata/visual/Diff*/.
//
// Fixture coverage (spec-1.13 D-8 § 4.2):
//
//	DiffSimple       — 1 hunk, 3 lines (1 added + 1 removed + 1 context)
//	DiffMultiHunk    — 3 hunks across same file
//	DiffGoCode       — hunk with Go body lang highlight + marker overlay
//	DiffPlainText    — hunk with no lang (plain marker color only)
//	DiffBareLines    — markdown ```diff fence path (no @@ header, no gutter)
//
// CODE_VISUAL_REQUIRED-style env gate: DIFF_VISUAL_REQUIRED=1 turns
// missing fixtures into fatal (CI strict); without it tests t.Skip
// (developer-local convenience). 7th independent block visual env gate.
func TestDiffVisualGolden(t *testing.T) {
	cases := []struct {
		name string
		diff block.Diff
		cols int
	}{
		{
			name: "DiffSimple",
			diff: block.NewDiffFromHunks([]block.Hunk{{
				OldStart: 1, OldLines: 2, NewStart: 1, NewLines: 2,
				Lines: []block.LineEntry{
					{Marker: ' ', Text: "context"},
					{Marker: '-', Text: "old"},
					{Marker: '+', Text: "new"},
				},
			}}),
			cols: 80,
		},
		{
			name: "DiffMultiHunk",
			diff: block.NewDiffFromHunks([]block.Hunk{
				{OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 1,
					Lines: []block.LineEntry{{Marker: '+', Text: "added in hunk 1"}}},
				{OldStart: 10, OldLines: 1, NewStart: 10, NewLines: 1,
					Lines: []block.LineEntry{{Marker: '-', Text: "removed in hunk 2"}}},
				{OldStart: 20, OldLines: 1, NewStart: 20, NewLines: 1,
					Lines: []block.LineEntry{{Marker: ' ', Text: "context in hunk 3"}}},
			}),
			cols: 80,
		},
		{
			name: "DiffGoCode",
			diff: func() block.Diff {
				d := block.NewDiffFromHunks([]block.Hunk{{
					OldStart: 1, OldLines: 3, NewStart: 1, NewLines: 3,
					Lines: []block.LineEntry{
						{Marker: ' ', Text: "package main"},
						{Marker: '-', Text: "func old() {}"},
						{Marker: '+', Text: "func new() string { return \"hi\" }"},
					},
				}})
				d.BodyLang = "go"
				d.FilePath = "main.go"
				return d
			}(),
			cols: 80,
		},
		{
			name: "DiffPlainText",
			diff: block.NewDiffFromHunks([]block.Hunk{{
				OldStart: 5, OldLines: 2, NewStart: 5, NewLines: 2,
				Lines: []block.LineEntry{
					{Marker: '-', Text: "Hello, World"},
					{Marker: '+', Text: "Hello, opendbx"},
				},
			}}),
			cols: 80,
		},
		{
			name: "DiffBareLines",
			diff: block.NewDiffFromBareLines("+added line\n-removed line\n context line"),
			cols: 80,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel() // R2 NIT-4: subtests are independent (no shared mutable state).
			ctx := ctxDefault(tc.cols)
			buf, err := tc.diff.Render(ctx)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			raw := bufferANSI(t, buf)
			uiinvariant.CheckANSI(t, raw)
			png := visualgolden.Render(t, raw, visualgolden.DefaultTheme())
			fixturePath := visualFixturePath(t, tc.name, "golden.png")
			if !visualgolden.Update() && !visualFixtureExists(t, fixturePath) {
				if os.Getenv("DIFF_VISUAL_REQUIRED") != "" {
					t.Fatalf("missing CC visual fixture for %s (run capture SOP first)", tc.name)
				}
				t.Skipf("missing CC visual fixture for %s; spec-1.13 capture pending (set DIFF_VISUAL_REQUIRED=1 once captured)", tc.name)
			}
			visualgolden.CompareFile(t, fixturePath, png, 0.02)
		})
	}
}

// TestDiffVisualGolden_ParkedFixtures verifies the 5 fixture parking
// dirs exist before harness runs.
func TestDiffVisualGolden_ParkedFixtures(t *testing.T) {
	t.Parallel()
	required := []string{
		"DiffSimple",
		"DiffMultiHunk",
		"DiffGoCode",
		"DiffPlainText",
		"DiffBareLines",
	}
	for _, name := range required {
		path := visualFixturePath(t, name, "")
		path = strings.TrimSuffix(path, "/")
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			t.Errorf("parked fixture dir missing: %s (run spec-1.13 capture SOP)", path)
		}
	}
}
