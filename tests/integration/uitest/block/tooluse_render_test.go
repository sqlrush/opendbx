// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

//go:build !windows

package block_test

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	cases := []toolUseVisualCase{
		{
			name: "ToolUseQueued",
			tu:   block.NewToolUse("id1", "Bash", map[string]any{"command": "pwd"}),
			cols: 80,
			meta: toolUseFixtureMetadata{
				Adapter:      "bash",
				Prompt:       "Run several shell commands and capture the queued Bash tool use.",
				ToolUseState: "queued",
			},
		},
		{
			name: "ToolUseRunningGeneric",
			tu: withState(block.NewToolUse("id2", "MysteryTool", map[string]any{"foo": "bar"}),
				block.StateRunning),
			cols: 80,
			// Single-row generic text shows slightly higher macOS/Linux
			// freeze font rasterization drift than the other ToolUse cases.
			maxMismatchFraction: 0.02,
			meta: toolUseFixtureMetadata{
				Adapter:      "generic",
				Notes:        "Unknown tool fallback is fixture-derived from deterministic mock input.",
				Prompt:       "Mock fallback for an unknown tool name.",
				ToolUseState: "running",
			},
		},
		{
			name: "ToolUseRunningBash",
			tu: withState(block.NewToolUse("id3", "Bash", map[string]any{"command": "git status"}),
				block.StateRunning),
			cols: 80,
			meta: toolUseFixtureMetadata{
				Adapter:      "bash",
				Prompt:       "Run git status and capture the running Bash tool use.",
				ToolUseState: "running",
			},
		},
		{
			name: "ToolUseRunningRead",
			tu: withState(block.NewToolUse("id4", "Read", map[string]any{"path": "main.go"}),
				block.StateRunning),
			cols: 80,
			meta: toolUseFixtureMetadata{
				Adapter:      "read",
				Notes:        "Read running state uses deterministic render input because live Read is often instantaneous.",
				Prompt:       "Read main.go and capture the running Read tool use.",
				ToolUseState: "running",
			},
		},
		{
			name: "ToolUseWaitingPermission",
			tu: withState(block.NewToolUse("id5", "Bash", map[string]any{"command": "rm /tmp/x"}),
				block.StateWaitingPermission),
			cols: 80,
			meta: toolUseFixtureMetadata{
				Adapter:      "bash",
				Prompt:       "Run a permission-gated shell command and capture the waiting state.",
				ToolUseState: "waiting_permission",
			},
		},
		{
			name: "ToolUseRunningBashWithProgress",
			tu: withProgress(
				withState(block.NewToolUse("id6", "Bash", map[string]any{"command": "long_cmd"}),
					block.StateRunning),
				[]adapter.ProgressMessage{{ElapsedSeconds: 5, TotalLines: 100, TotalBytes: 4096}}),
			cols: 80,
			// Multi-row + progress text shows higher freeze font rasterization
			// drift than single-row cases (locally seen ~1.17% darwin/arm64);
			// same calibration pattern as ToolUseRunningGeneric.
			maxMismatchFraction: 0.02,
			meta: toolUseFixtureMetadata{
				Adapter:      "bash",
				Prompt:       "Run a long shell command and capture Bash progress output.",
				ToolUseState: "running",
			},
		},
		{
			name: "ToolUseResolvedRead",
			tu: withState(block.NewToolUse("id7", "Read", map[string]any{"path": "main.go"}),
				block.StateResolved),
			cols: 80,
			meta: toolUseFixtureMetadata{
				Adapter:      "read",
				Prompt:       "Read main.go and capture the resolved Read tool use.",
				ToolUseState: "resolved",
			},
		},
		// spec-2.3 D-4 (post-impl cr MED-2): Skill adapter Layer-2 cases.
		{
			name: "ToolUseRunningSkill",
			tu: withState(block.NewToolUse("id8", "Skill", map[string]any{"skill": "code-reviewer"}),
				block.StateRunning),
			cols: 80,
			meta: toolUseFixtureMetadata{
				Adapter:      "skill",
				Prompt:       "Invoke the code-reviewer skill and capture the running Skill tool use.",
				ToolUseState: "running",
			},
		},
		{
			name: "ToolUseResolvedSkillArgs",
			tu: withState(block.NewToolUse("id9", "Skill", map[string]any{
				"skill": "topsql",
				"args":  map[string]any{"db": "main", "limit": "10"},
			}), block.StateResolved),
			cols: 80,
			meta: toolUseFixtureMetadata{
				Adapter:      "skill",
				Notes:        "Args render as the sorted compact k=v summary after Skill(<name>).",
				Prompt:       "Invoke topsql with args and capture the resolved Skill tool use.",
				ToolUseState: "resolved",
			},
		},
		{
			name: "ToolUseRunningSkillNoName",
			tu: withState(block.NewToolUse("idA", "Skill", map[string]any{}),
				block.StateRunning),
			cols: 80,
			meta: toolUseFixtureMetadata{
				Adapter:      "skill",
				Notes:        "Missing skill field renders the Skill(no skill) placeholder.",
				Prompt:       "Mock a Skill call without a skill field (placeholder fallback).",
				ToolUseState: "running",
			},
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
			_, rows := buf.Size()
			uiinvariant.CheckANSI(t, raw)
			png := visualgolden.Render(t, raw, visualgolden.DefaultTheme())
			fixturePath := visualFixturePath(t, tc.name, "golden.png")
			if !visualgolden.Update() && !visualFixtureExists(t, fixturePath) {
				// spec-1.9 D-9: ToolUse fixtures use a separate env gate
				// from spec-1.7 Message fixtures so that turning
				// BLOCK_VISUAL_REQUIRED=1 for Message doesn't fatal on
				// ToolUse fixtures that haven't been captured yet.
				// Set TOOLUSE_VISUAL_REQUIRED=1 once ToolUse capture lands.
				if os.Getenv("TOOLUSE_VISUAL_REQUIRED") != "" {
					t.Fatalf("missing CC visual fixture for %s (run T-2.5 capture SOP first)", tc.name)
				}
				t.Skipf("missing CC visual fixture for %s; T-2.5 ToolUse capture pending (set TOOLUSE_VISUAL_REQUIRED=1 once captured)", tc.name)
			}
			if visualgolden.Update() {
				writeToolUseFixtureSidecars(t, tc, raw, rows)
			}
			visualgolden.CompareFile(t, fixturePath, png, tc.maxMismatch())
		})
	}
}

type toolUseFixtureMetadata struct {
	Adapter      string
	Notes        string
	Prompt       string
	ToolUseState string
}

type toolUseVisualCase struct {
	name                string
	tu                  block.ToolUse
	cols                int
	maxMismatchFraction float64
	meta                toolUseFixtureMetadata
}

func (tc toolUseVisualCase) maxMismatch() float64 {
	if tc.maxMismatchFraction != 0 {
		return tc.maxMismatchFraction
	}
	return 0.01
}

func writeToolUseFixtureSidecars(t testing.TB, tc toolUseVisualCase, raw []byte, rows int) {
	t.Helper()
	dir := visualFixturePath(t, tc.name, "")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir tooluse fixture dir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "input.ansi"), raw, 0o600); err != nil {
		t.Fatalf("write tooluse input.ansi: %v", err)
	}
	meta := map[string]any{
		"adapter":           tc.meta.Adapter,
		"captured_at":       "2026-05-23T00:00:00Z",
		"cc_version":        "2.1.148",
		"cols":              tc.cols,
		"fixture":           tc.name,
		"font":              "freeze default monospace",
		"golden_source":     "opendbx-rendered-block-tooluse",
		"input_ansi_source": "opendbx-rendered-block-tooluse",
		"notes":             tc.meta.Notes,
		"prompt":            tc.meta.Prompt,
		"rows":              rows,
		"sanitizer":         "v1 - no PII, generated from deterministic fixture text",
		"terminal":          "automated visualgolden.Render baseline; CC source-fidelity audit reviewed separately",
		"theme":             "opendbx DefaultTheme; CC ToolUse fixture lock-in pending for transient states",
		"tool_use_state":    tc.meta.ToolUseState,
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		t.Fatalf("marshal tooluse metadata: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), data, 0o600); err != nil {
		t.Fatalf("write tooluse metadata.json: %v", err)
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
		// spec-2.3 D-4 Skill adapter (capture pending per SOP).
		"ToolUseRunningSkill",
		"ToolUseResolvedSkillArgs",
		"ToolUseRunningSkillNoName",
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
