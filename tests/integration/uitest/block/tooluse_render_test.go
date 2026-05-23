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
	"github.com/sqlrush/opendbx/internal/app/cli/render/block/adapter"
	"github.com/sqlrush/opendbx/internal/testing/uiinvariant"
	"github.com/sqlrush/opendbx/internal/testing/visualgolden"
)

// TestToolUseVisualGolden consumes the 7 ToolUse CC fixtures captured
// during the spec-1.7 T-2.5 session (parked under
// tests/integration/uitest/block/testdata/visual/ToolUse*/).
//
// Fixture coverage (spec-1.9 D-9 § 4.2 INT-2):
//
//	ToolUseQueued                  — Bash, queued
//	ToolUseRunningGeneric          — unknown tool name → Generic fallback
//	ToolUseRunningBash             — Bash "git status" running, no progress
//	ToolUseRunningRead             — Read "main.go", no ProgressRenderer
//	ToolUseWaitingPermission       — Bash awaiting approval
//	ToolUseRunningBashWithProgress — Bash + 5s elapsed + 100-line progress
//	ToolUseResolvedRead            — Read resolved, single header row
//
// BLOCK_VISUAL_REQUIRED=1 turns missing fixtures into fatal (CI strict);
// without it tests t.Skip (developer-local convenience).
func TestToolUseVisualGolden(t *testing.T) {
	cases := []struct {
		name string
		tu   block.ToolUse
		cols int
	}{
		{
			name: "ToolUseQueued",
			tu:   block.NewToolUse("id1", "Bash", map[string]any{"command": "pwd"}),
			cols: 80,
		},
		{
			name: "ToolUseRunningGeneric",
			tu: withState(block.NewToolUse("id2", "MysteryTool", map[string]any{"foo": "bar"}),
				block.StateRunning),
			cols: 80,
		},
		{
			name: "ToolUseRunningBash",
			tu: withState(block.NewToolUse("id3", "Bash", map[string]any{"command": "git status"}),
				block.StateRunning),
			cols: 80,
		},
		{
			name: "ToolUseRunningRead",
			tu: withState(block.NewToolUse("id4", "Read", map[string]any{"path": "main.go"}),
				block.StateRunning),
			cols: 80,
		},
		{
			name: "ToolUseWaitingPermission",
			tu: withState(block.NewToolUse("id5", "Bash", map[string]any{"command": "rm /tmp/x"}),
				block.StateWaitingPermission),
			cols: 80,
		},
		{
			name: "ToolUseRunningBashWithProgress",
			tu: withProgress(
				withState(block.NewToolUse("id6", "Bash", map[string]any{"command": "long_cmd"}),
					block.StateRunning),
				[]adapter.ProgressMessage{{ElapsedSeconds: 5, TotalLines: 100, TotalBytes: 4096}}),
			cols: 80,
		},
		{
			name: "ToolUseResolvedRead",
			tu: withState(block.NewToolUse("id7", "Read", map[string]any{"path": "main.go"}),
				block.StateResolved),
			cols: 80,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx := ctxDefault(tc.cols)
			buf, err := tc.tu.Render(ctx)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			raw := bufferANSI(t, buf)
			uiinvariant.CheckANSI(t, raw)
			png := visualgolden.Render(t, raw, visualgolden.DefaultTheme())
			fixturePath := visualFixturePath(t, tc.name, "golden.png")
			if !visualgolden.Update() && !visualFixtureExists(t, fixturePath) {
				if os.Getenv("BLOCK_VISUAL_REQUIRED") != "" {
					t.Fatalf("missing CC visual fixture for %s (run T-2.5 capture SOP first)", tc.name)
				}
				t.Skipf("missing CC visual fixture for %s; T-2.5 capture pending", tc.name)
			}
			visualgolden.CompareFile(t, fixturePath, png, 0.01)
		})
	}
}

// withState is a small builder helper for table-driven test cases.
func withState(tu block.ToolUse, s block.ToolUseState) block.ToolUse {
	tu.State = s
	return tu
}

// withProgress attaches ProgressMessages to a ToolUse value.
func withProgress(tu block.ToolUse, msgs []adapter.ProgressMessage) block.ToolUse {
	tu.ProgressMessages = msgs
	return tu
}

// TestToolUseVisualGolden_ParkedFixtures verifies the 7 fixture parking
// dirs exist (created by spec-1.7 T-2.5 SOP scaffolding). Failing this
// before the harness runs means CAPTURE_SOP.md instructions for the
// dir layout are stale or the dir was deleted.
func TestToolUseVisualGolden_ParkedFixtures(t *testing.T) {
	t.Parallel()
	required := []string{
		"ToolUseQueued",
		"ToolUseRunningGeneric",
		"ToolUseRunningBash",
		"ToolUseRunningRead",
		"ToolUseWaitingPermission",
		"ToolUseRunningBashWithProgress",
		"ToolUseResolvedRead",
	}
	for _, name := range required {
		path := visualFixturePath(t, name, "")
		path = strings.TrimSuffix(path, "/")
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			t.Errorf("parked fixture dir missing: %s (run T-2.5 SOP / spec-1.7 capture)", path)
		}
	}
}
