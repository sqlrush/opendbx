// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

//go:build !windows

// Package diagnoseloop_test holds the spec-1.21 D-8 integration smoke +
// 12th independent block-style visual fixture parking (env gate
// DIAGNOSELOOP_VISUAL_REQUIRED). Tests drive llmapp.Model via the same
// keystroke + Update + Cmd-chain path that program.Run uses in
// production — so a regression in the bootstrap-equivalent wiring
// (Registry → llmapp.Options → diagnose.NewLoop → makeEmit → handleControl)
// surfaces here, not at first interact session.
package diagnoseloop_test

import (
	"bytes"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/keybindings"
	"github.com/sqlrush/opendbx/internal/app/cli/llmapp"
	"github.com/sqlrush/opendbx/internal/app/cli/program"
	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
	"github.com/sqlrush/opendbx/internal/app/cli/render/terminal"
	"github.com/sqlrush/opendbx/internal/app/diagnose"
	"github.com/sqlrush/opendbx/internal/domain/llm"
	"github.com/sqlrush/opendbx/internal/domain/llm/fake"
)

// productionRegistry mirrors bootstrap.defaultDiagnoseRegistry: a Registry
// of read-only clock + echo. Defined here so integration tests don't have
// to import the internal/bootstrap package (which would build a TUI screen).
func productionRegistry(t *testing.T) *diagnose.Registry {
	t.Helper()
	reg, err := diagnose.NewRegistry(diagnose.ClockTool{}, diagnose.EchoTool{})
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	return reg
}

// drive submits "ask" through a real llmapp.Model and drains the Cmd
// chain to terminal — the same input pipeline program.Run drives in
// production. Returns the final Model so tests can assert scrollback /
// state shape.
func drive(t *testing.T, prov llm.Provider, opts llmapp.Options) *llmapp.Model {
	t.Helper()
	m := llmapp.New(prov, opts)
	cur := program.Model(m)
	for _, r := range "ask" {
		cur, _ = cur.Update(program.KeyActionMsg{Key: program.KeyMsg{Code: terminal.KeyRune, Rune: r}, Action: keybindings.ActionInsertRune})
	}
	cur, cmd := cur.Update(program.KeyActionMsg{Key: program.KeyMsg{Code: terminal.KeyEnter}, Action: keybindings.ActionSubmit})
	for cmd != nil {
		msg := cmd()
		if msg == nil {
			break
		}
		cur, cmd = cur.Update(msg)
	}
	mm := cur.(*llmapp.Model)
	_ = mm.View(80, 24) // final Drain
	return mm
}

// scrollbackKinds is a thin wrapper around Model.ScrollbackTypesForTest
// (the test seam exposed by llmapp for this exact assertion). Indirected
// through a local function so a future swap to an event-recorder seam
// is one edit away.
func scrollbackKinds(m *llmapp.Model) []string {
	return m.ScrollbackTypesForTest()
}

// ============================================================
// Full-loop smoke: prompt → ToolUse block → ToolResult block → final
// ============================================================

// TestDiagnoseLoop_FullSmoke is the headline T-9 acceptance: a model
// scripted to call echo once and then stop renders the proper
// block.ToolUse + block.ToolResult render nodes (NOT the retired
// "spec-1.21" placeholder) and surfaces the final assistant text. The
// scrollback shape mirrors what a real interact session will show
// when the production Registry is wired (bootstrap.defaultDiagnoseRegistry).
func TestDiagnoseLoop_FullSmoke(t *testing.T) {
	t.Parallel()
	reg := productionRegistry(t)
	prov := fake.NewScriptedTurns(
		fake.Turn{
			ToolUses: []llm.ToolUse{{ID: "c1", Name: "echo", Input: map[string]any{"msg": "ping"}}},
			Finish:   llm.FinishToolUse,
		},
		fake.Turn{Text: "echo done", Finish: llm.FinishStop},
	)
	final := drive(t, prov, llmapp.Options{
		ModelName: "fake", MaxTokens: 1024, Registry: reg,
	})

	// Expected scrollback shape: [user Message, ToolUse, ToolResult, final Message].
	kinds := scrollbackKinds(final)
	want := []string{
		"block.Message",    // user echo line ("> ask")
		"block.ToolUse",    // EventToolCall handler appended this
		"block.ToolResult", // EventToolResult handler appended this
		"block.Message",    // assistant final text ("echo done")
	}
	if len(kinds) != len(want) {
		t.Fatalf("scrollback shape = %v; want %v", kinds, want)
	}
	for i, k := range want {
		if kinds[i] != k {
			t.Errorf("scrollback[%d] = %s; want %s", i, kinds[i], k)
		}
	}
	if prov.CallCount() != 2 {
		t.Errorf("provider call count = %d; want 2 (one per loop turn)", prov.CallCount())
	}
	// Final answer text reached scrollback.
	grid := final.View(80, 24)
	body := string(gridASCII(grid))
	if !strings.Contains(body, "echo done") {
		t.Errorf("final assistant text not in view: %q", body)
	}
	// And the retired placeholder text MUST NOT appear.
	for _, banned := range []string{"执行待 spec-1.21", "spec-1.21]"} {
		if strings.Contains(body, banned) {
			t.Errorf("retired placeholder leaked into view: %q in %q", banned, body)
		}
	}
	// codex T-10a P2-1 — the direct ToolUse.State Resolved/Error
	// assertion lives at the unit level in llmapp/model_test.go
	// (TestModel_LoopTransitionsToolUseState / *_ToError) where the
	// scrollback node is reachable. The integration seam exposes only
	// node type names; surfacing state would widen the seam unnecessarily.
}

// ============================================================
// 1.20 no-tool regression: bare chat unchanged by spec-1.21 wiring
// ============================================================

// TestDiagnoseLoop_NoToolRegression confirms a 1-turn FinishStop text
// reply behaves byte-equivalent to the spec-1.20 single-stream path:
// CallCount=1, scrollback = [user Message, assistant Message], no
// orphan render nodes from the loop machinery.
func TestDiagnoseLoop_NoToolRegression(t *testing.T) {
	t.Parallel()
	prov := fake.NewScriptedTurns(
		fake.Turn{Text: "hello, no tool here", Finish: llm.FinishStop},
	)
	// Registry intentionally nil — bare chat path.
	final := drive(t, prov, llmapp.Options{ModelName: "fake", MaxTokens: 1024})

	kinds := scrollbackKinds(final)
	want := []string{"block.Message", "block.Message"}
	if !reflect.DeepEqual(kinds, want) {
		t.Errorf("scrollback shape = %v; want %v (1.20 no-tool regression)", kinds, want)
	}
	if prov.CallCount() != 1 {
		t.Errorf("provider call count = %d; want 1 (single-turn 1.20 shape)", prov.CallCount())
	}
	grid := final.View(80, 24)
	body := string(gridASCII(grid))
	if !strings.Contains(body, "hello, no tool here") {
		t.Errorf("assistant text missing from view: %q", body)
	}
}

// ============================================================
// Unknown-tool: production registry path still renders TOOL_UNKNOWN
// ============================================================

// TestDiagnoseLoop_UnknownToolRendersMarker covers the case where the
// model requests a name not in the production Registry (e.g. a future
// model hallucinating "topsql" before spec-2.1 ships). The Loop
// terminates with DIAGNOSE.TOOL_UNKNOWN and the marker reaches the UI;
// no orphan ToolUse block is committed.
func TestDiagnoseLoop_UnknownToolRendersMarker(t *testing.T) {
	t.Parallel()
	reg := productionRegistry(t) // contains clock + echo, NOT topsql
	prov := fake.NewScriptedTurns(fake.Turn{
		ToolUses: []llm.ToolUse{{ID: "g", Name: "topsql"}},
		Finish:   llm.FinishToolUse,
	})
	final := drive(t, prov, llmapp.Options{ModelName: "fake", MaxTokens: 1024, Registry: reg})

	body := string(gridASCII(final.View(80, 24)))
	if !strings.Contains(body, "DIAGNOSE.TOOL_UNKNOWN") {
		t.Errorf("missing DIAGNOSE.TOOL_UNKNOWN marker: %q", body)
	}
	// scrollback must NOT contain a block.ToolUse for the orphan call.
	for _, k := range scrollbackKinds(final) {
		if k == "block.ToolUse" {
			t.Errorf("orphan block.ToolUse committed despite unknown tool: %v", scrollbackKinds(final))
		}
	}
}

// ============================================================
// 12th visual fixture parking (DIAGNOSELOOP_VISUAL_REQUIRED env gate)
// ============================================================

// TestDiagnoseLoopVisualGolden_ParkedFixtures parks the 12th independent
// block-style visual fixture set per spec-1.21 D-8. Each fixture dir
// must exist; golden.png capture lands in a follow-up SOP. The env gate
// DIAGNOSELOOP_VISUAL_REQUIRED=1 turns missing dirs into failures (CI
// strict) — same pattern as spec-1.10..1.20.
func TestDiagnoseLoopVisualGolden_ParkedFixtures(t *testing.T) {
	t.Parallel()
	fixtures := []string{
		"DiagnoseLoopHappyToolRoundTrip",
		"DiagnoseLoopUnknownTool",
		"DiagnoseLoopNoToolRegression",
	}
	for _, name := range fixtures {
		path := filepath.Join("testdata", "visual", name)
		_, err := visualStat(path)
		if err != nil {
			t.Errorf("parked fixture dir missing: %s (run spec-1.21 capture SOP)", path)
		}
	}
}

// --- harness helpers ---

func gridASCII(buf buffer.Buffer) []byte {
	if buf == nil {
		return nil
	}
	cols, rows := buf.Size()
	var b bytes.Buffer
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			c := buf.Cell(x, y)
			if c.Ch <= 0 {
				b.WriteByte(' ')
				continue
			}
			b.WriteRune(c.Ch)
		}
		b.WriteByte('\n')
	}
	return b.Bytes()
}
