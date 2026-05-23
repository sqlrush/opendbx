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

// TestToolResultVisualGolden consumes 6 ToolResult CC fixtures parked
// under tests/integration/uitest/block/testdata/visual/ToolResult*/.
//
// Fixture coverage (spec-1.9b D-8 § 4.2 INT-2):
//
//	ToolResultSuccessBash   — Bash success output
//	ToolResultSuccessRead   — Read N lines
//	ToolResultErrorBash     — Bash non-zero exit
//	ToolResultErrorRead     — Read file not found
//	ToolResultRejectedBash  — Bash user-rejected sensitive command
//	ToolResultCanceledBash  — Bash Ctrl+C mid-run
//
// TOOLRESULT_VISUAL_REQUIRED=1 turns missing fixtures into fatal (CI
// strict); without it tests t.Skip (developer-local convenience).
// Independent env gate per spec-1.9b D-8 lesson (decoupled from
// BLOCK_VISUAL_REQUIRED + TOOLUSE_VISUAL_REQUIRED).
func TestToolResultVisualGolden(t *testing.T) {
	cases := []struct {
		name string
		tr   block.ToolResult
		cols int
	}{
		{
			name: "ToolResultSuccessBash",
			tr:   block.NewToolResult("id1", "Bash", "Hello\nWorld", false),
			cols: 80,
		},
		{
			name: "ToolResultSuccessRead",
			tr:   block.NewToolResult("id2", "Read", "line1\nline2\nline3\nline4\nline5", false),
			cols: 80,
		},
		{
			name: "ToolResultErrorBash",
			tr:   block.NewToolResult("id3", "Bash", "exit 127: command not found", true),
			cols: 80,
		},
		{
			name: "ToolResultErrorRead",
			tr:   block.NewToolResult("id4", "Read", "File not found: /tmp/missing.txt", true),
			cols: 80,
		},
		{
			name: "ToolResultRejectedBash",
			tr:   buildRejectedToolResult("id5"),
			cols: 80,
		},
		{
			name: "ToolResultCanceledBash",
			tr:   buildCanceledToolResult("id6"),
			cols: 80,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx := ctxDefault(tc.cols)
			buf, err := tc.tr.Render(ctx)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			raw := bufferANSI(t, buf)
			uiinvariant.CheckANSI(t, raw)
			png := visualgolden.Render(t, raw, visualgolden.DefaultTheme())
			fixturePath := visualFixturePath(t, tc.name, "golden.png")
			if !visualgolden.Update() && !visualFixtureExists(t, fixturePath) {
				// spec-1.9b D-8: independent TOOLRESULT_VISUAL_REQUIRED env
				// gate (decoupled from BLOCK_VISUAL_REQUIRED for Message
				// and TOOLUSE_VISUAL_REQUIRED for ToolUse).
				if os.Getenv("TOOLRESULT_VISUAL_REQUIRED") != "" {
					t.Fatalf("missing CC visual fixture for %s (run T-2.5 capture SOP first)", tc.name)
				}
				t.Skipf("missing CC visual fixture for %s; T-2.5 ToolResult capture pending (set TOOLRESULT_VISUAL_REQUIRED=1 once captured)", tc.name)
			}
			visualgolden.CompareFile(t, fixturePath, png, 0.02)
		})
	}
}

// buildRejectedToolResult constructs a Rejected ToolResult using a
// prefix substring of the unexported block.rejectMessagePrefix const
// (matches the first sentence of CC messages.ts:210 REJECT_MESSAGE).
// Since `deriveResultState` uses `strings.HasPrefix(content,
// rejectMessagePrefix)`, our prefix-substring also matches because the
// full REJECT_MESSAGE starts with this string.
//
// NIT-1 (R3): substring used because test file (block_test package)
// cannot import unexported const from block package. Future
// alternative: expose a test helper `block.ForTesting_RejectMessagePrefix`.
func buildRejectedToolResult(id string) block.ToolResult {
	prefix := "The user doesn't want to proceed with this tool use."
	return block.NewToolResult(id, "Bash", prefix, false)
}

// buildCanceledToolResult constructs a Canceled ToolResult using a
// prefix substring of the unexported block.cancelMessagePrefix const
// (matches CC messages.ts:207 CANCEL_MESSAGE first sentence).
// Same prefix-substring rationale as buildRejectedToolResult.
func buildCanceledToolResult(id string) block.ToolResult {
	prefix := "The user doesn't want to take this action right now."
	return block.NewToolResult(id, "Bash", prefix, false)
}

// TestToolResultVisualGolden_ParkedFixtures verifies the 6 fixture
// parking dirs exist before harness runs.
func TestToolResultVisualGolden_ParkedFixtures(t *testing.T) {
	t.Parallel()
	required := []string{
		"ToolResultSuccessBash",
		"ToolResultSuccessRead",
		"ToolResultErrorBash",
		"ToolResultErrorRead",
		"ToolResultRejectedBash",
		"ToolResultCanceledBash",
	}
	for _, name := range required {
		path := visualFixturePath(t, name, "")
		path = strings.TrimSuffix(path, "/")
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			t.Errorf("parked fixture dir missing: %s (run T-2.5 SOP / spec-1.9b capture)", path)
		}
	}
}
