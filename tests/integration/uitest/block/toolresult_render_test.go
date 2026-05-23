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

const toolResultRejectMessage = "The user doesn't want to proceed with this tool use. The tool use was rejected (eg. if it was a file edit, the new_string was NOT written to the file). STOP what you are doing and wait for the user to tell you how to proceed."

const toolResultCancelMessage = "The user doesn't want to take this action right now. STOP what you are doing and wait for the user to tell you how to proceed."

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
			tr:   buildRejectedToolResult(t, "id5"),
			cols: 80,
		},
		{
			name: "ToolResultCanceledBash",
			tr:   buildCanceledToolResult(t, "id6"),
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

// buildRejectedToolResult constructs a Rejected ToolResult using the full
// CC REJECT_MESSAGE literal. The state assertion prevents the visual fixture
// from silently covering Success when the sentence drifts.
func buildRejectedToolResult(t *testing.T, id string) block.ToolResult {
	t.Helper()
	tr := block.NewToolResult(id, "Bash", toolResultRejectMessage, false)
	if tr.State != block.ResultRejected {
		t.Fatalf("Rejected fixture state: got %v, want %v", tr.State, block.ResultRejected)
	}
	return tr
}

// buildCanceledToolResult constructs a Canceled ToolResult using the full
// CC CANCEL_MESSAGE literal and asserts the intended derived state.
func buildCanceledToolResult(t *testing.T, id string) block.ToolResult {
	t.Helper()
	tr := block.NewToolResult(id, "Bash", toolResultCancelMessage, false)
	if tr.State != block.ResultCanceled {
		t.Fatalf("Canceled fixture state: got %v, want %v", tr.State, block.ResultCanceled)
	}
	return tr
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
