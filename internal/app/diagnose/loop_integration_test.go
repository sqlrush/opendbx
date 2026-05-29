// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File loop_integration_test.go — spec-1.21 T-7 full-loop integration.
//
// These tests exercise diagnose.Loop end-to-end through the real
// fake.Provider (multi-turn ScriptedTurns) — proving the orchestrator
// is not just "unit-correct" but actually drives a provider through a
// complete user → tool_use → executor → tool_result → final-answer
// round-trip, with paired-history invariants enforced.
//
// Why a separate file: the unit tests in loop_test.go use an inline
// stub provider that bypasses llm.ValidateRequest and the chunk-by-
// chunk delivery shape. fake.Provider IS the validation surface a real
// adapter shows; running the loop against it guards us from regressions
// where the unit-stub's leniency masks a contract drift.

package diagnose

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sqlrush/opendbx/internal/domain/llm"
	"github.com/sqlrush/opendbx/internal/domain/llm/fake"
)

// fixedClockNow returns a deterministic timestamp used across integration
// tests so the committed ToolResult.Content is byte-stable.
func fixedClockNow() time.Time {
	return time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
}

// assertPairedHistory walks msgs and verifies the spec-1.21 invariant
// the user T-7 brief calls out: every assistant turn carrying tool_use
// blocks is immediately followed by a user turn carrying matching
// tool_result blocks, paired by ID, in order, with no orphans either way.
//
// The walker tolerates leading user input + trailing assistant text
// (the final answer), so a well-formed transcript like
//
//	[user, assistant{tool_use c1}, user{tool_result c1}, assistant{text}]
//
// passes. An orphan tool_use OR an orphan tool_result fails the test.
func assertPairedHistory(t *testing.T, msgs []llm.Message) {
	t.Helper()
	i := 0
	for i < len(msgs) {
		m := msgs[i]
		toolUses := collectTypeIDs(m.Content, llm.BlockToolUse)
		if len(toolUses) == 0 {
			i++
			continue
		}
		// This assistant message has tool_use blocks. The next message
		// MUST exist and be a user turn whose tool_result IDs match
		// exactly (order preserved).
		if m.Role != llm.RoleAssistant {
			t.Fatalf("msg %d: tool_use blocks appear on role=%v; expected RoleAssistant", i, m.Role)
		}
		if i+1 >= len(msgs) {
			t.Fatalf("msg %d (assistant tool_use) has no following user tool_result — orphan tool_use (provider would 400 on resume)", i)
		}
		next := msgs[i+1]
		if next.Role != llm.RoleUser {
			t.Fatalf("msg %d (assistant tool_use) followed by role=%v; expected RoleUser tool_result", i, next.Role)
		}
		toolResults := collectTypeIDs(next.Content, llm.BlockToolResult)
		if len(toolResults) != len(toolUses) {
			t.Fatalf("msg %d: %d tool_use → msg %d: %d tool_result; want equal count (paired-commit invariant)", i, len(toolUses), i+1, len(toolResults))
		}
		for k := range toolUses {
			if toolUses[k] != toolResults[k] {
				t.Fatalf("msg %d/%d: tool_use[%d].ID=%q vs tool_result[%d].ID=%q — IDs must match in order", i, i+1, k, toolUses[k], k, toolResults[k])
			}
		}
		i += 2
	}
}

// collectTypeIDs returns the IDs (ToolUse.ID or ToolResult.ToolUseID) of
// every content block of the given Type. Order is preserved.
func collectTypeIDs(blocks []llm.ContentBlock, want llm.BlockType) []string {
	ids := []string{}
	for _, b := range blocks {
		if b.Type != want {
			continue
		}
		switch want {
		case llm.BlockToolUse:
			if b.ToolUse != nil {
				ids = append(ids, b.ToolUse.ID)
			}
		case llm.BlockToolResult:
			if b.ToolResult != nil {
				ids = append(ids, b.ToolResult.ToolUseID)
			}
		}
	}
	return ids
}

// ============================================================
// Full-loop happy path: user → tool_use → executor → tool_result → final
// ============================================================

func TestIntegration_FullLoop_HappyToolRoundTrip(t *testing.T) {
	t.Parallel()
	reg, err := NewRegistry(ClockTool{Now: fixedClockNow}, EchoTool{})
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	prov := fake.NewScriptedTurns(
		// Turn 1: assistant asks for clock.
		fake.Turn{
			Text:     "let me check",
			ToolUses: []llm.ToolUse{{ID: "c1", Name: "clock"}},
			Finish:   llm.FinishToolUse,
		},
		// Turn 2: assistant integrates the tool_result and stops.
		fake.Turn{Text: "now is " + fixedClockNow().UTC().Format(time.RFC3339), Finish: llm.FinishStop},
	)

	loop, err := NewLoop(Options{Provider: prov, Registry: reg})
	if err != nil {
		t.Fatalf("NewLoop: %v", err)
	}
	r := &recorder{}
	res, runErr := loop.Run(context.Background(), userReq("when"), r.emit)
	if runErr != nil {
		t.Fatalf("Run err: %v", runErr)
	}
	if res.FinishReason != llm.FinishStop || res.TermCode != "" || res.Turns != 2 {
		t.Errorf("Result = %+v; want FinishStop / empty / 2", res)
	}
	if prov.CallCount() != 2 {
		t.Errorf("provider call count = %d; want 2 (one per turn)", prov.CallCount())
	}

	// Paired-history invariant: no orphan tool_use / tool_result.
	assertPairedHistory(t, res.Messages)

	// Spot-check the round-trip data flow: user input is msg[0]; assistant
	// tool_use msg[1] carries clock; user tool_result msg[2] carries the
	// fixed-clock RFC3339 string; assistant final text is NOT a separate
	// message (the second turn ended with FinishStop and no tool_use, so
	// the loop does not commit it to msgs — Result.Messages is the
	// wire transcript, not the rendered transcript).
	if len(res.Messages) != 3 {
		t.Fatalf("Messages = %d; want 3 (user + asst tool_use + user tool_result)", len(res.Messages))
	}
	asst := res.Messages[1]
	if asst.Role != llm.RoleAssistant || len(asst.Content) != 2 ||
		asst.Content[0].Type != llm.BlockText || asst.Content[0].Text != "let me check" ||
		asst.Content[1].Type != llm.BlockToolUse {
		t.Errorf("assistant turn shape wrong: %+v", asst)
	}
	user := res.Messages[2]
	if user.Role != llm.RoleUser || len(user.Content) != 1 ||
		user.Content[0].Type != llm.BlockToolResult {
		t.Errorf("user tool_result turn shape wrong: %+v", user)
	}
	if got := user.Content[0].ToolResult.Content; got != "2026-05-29T12:00:00Z" {
		t.Errorf("tool_result.Content = %q; want clock RFC3339", got)
	}

	// Event sequence — TurnStart×2 + 1 Text + ToolCall + ToolResult +
	// final Text + Finish (7 events).
	wantKinds := []EventKind{
		EventTurnStart, EventText, EventToolCall, EventToolResult,
		EventTurnStart, EventText, EventFinish,
	}
	if len(r.events) != len(wantKinds) {
		t.Fatalf("event count = %d; want %d (%+v)", len(r.events), len(wantKinds), r.events)
	}
	for i, k := range wantKinds {
		if r.events[i].Kind != k {
			t.Errorf("event[%d].Kind = %v; want %v", i, r.events[i].Kind, k)
		}
	}
}

// ============================================================
// 1.20 single-turn chat regression: no-tool path stays the 1.20 shape
// ============================================================

// TestIntegration_Spec120Regression_NoToolSingleTurn verifies that
// running Loop with a registry-less Options against a provider that
// emits a plain text + FinishStop in a single turn is byte-equivalent
// to the spec-1.20 single-turn chat: exactly one provider call, zero
// committed transcript additions (only the caller's input remains),
// FinishStop terminal. This is the "1.21 doesn't break 1.20" guard.
func TestIntegration_Spec120Regression_NoToolSingleTurn(t *testing.T) {
	t.Parallel()
	prov := fake.NewScriptedTurns(
		fake.Turn{Text: "Hello — VACUUM keeps PostgreSQL tables tidy.", Finish: llm.FinishStop},
	)
	loop, err := NewLoop(Options{Provider: prov /* no Registry → no tools */})
	if err != nil {
		t.Fatalf("NewLoop: %v", err)
	}
	r := &recorder{}
	res, runErr := loop.Run(context.Background(), userReq("explain VACUUM"), r.emit)
	if runErr != nil {
		t.Fatalf("Run err: %v", runErr)
	}
	if res.FinishReason != llm.FinishStop || res.TermCode != "" || res.Turns != 1 {
		t.Errorf("Result = %+v; want FinishStop / empty / 1 (1.20 regression)", res)
	}
	if prov.CallCount() != 1 {
		t.Errorf("provider call count = %d; want 1 (1.20 single-turn)", prov.CallCount())
	}
	if len(res.Messages) != 1 {
		t.Errorf("Messages = %d; want 1 (caller input only — no committed assistant turn for plain chat)", len(res.Messages))
	}

	// Paired invariant is trivially satisfied (no tool_use anywhere), but
	// run it anyway so a future regression that accidentally emits an
	// orphan still fires here.
	assertPairedHistory(t, res.Messages)

	// Event sequence — TurnStart + Text + Finish (3 events; same as
	// spec-1.20 single-turn shape, no tool events at all).
	wantKinds := []EventKind{EventTurnStart, EventText, EventFinish}
	if len(r.events) != len(wantKinds) {
		t.Fatalf("event count = %d; want %d (1.20 shape)", len(r.events), len(wantKinds))
	}
	for i, k := range wantKinds {
		if r.events[i].Kind != k {
			t.Errorf("event[%d].Kind = %v; want %v (1.20 shape)", i, r.events[i].Kind, k)
		}
	}
}

// ============================================================
// Multi-tool turn: assistant requests N tools in one turn
// ============================================================

// TestIntegration_FullLoop_MultiToolPerTurn covers the spec-1.21 D-4
// step 4 "顺序执行" path: the assistant requests TWO tools in a single
// turn; both execute, both ToolResults appear in the SAME user turn,
// and the IDs round-trip exactly in order.
func TestIntegration_FullLoop_MultiToolPerTurn(t *testing.T) {
	t.Parallel()
	reg, _ := NewRegistry(ClockTool{Now: fixedClockNow}, EchoTool{})
	prov := fake.NewScriptedTurns(
		fake.Turn{
			ToolUses: []llm.ToolUse{
				{ID: "c1", Name: "clock"},
				{ID: "c2", Name: "echo", Input: map[string]any{"msg": "ping"}},
			},
			Finish: llm.FinishToolUse,
		},
		fake.Turn{Text: "done", Finish: llm.FinishStop},
	)
	loop, _ := NewLoop(Options{Provider: prov, Registry: reg})
	res, err := loop.Run(context.Background(), userReq("do both"), (&recorder{}).emit)
	if err != nil {
		t.Fatalf("Run err: %v", err)
	}
	assertPairedHistory(t, res.Messages)
	if len(res.Messages) != 3 {
		t.Fatalf("Messages = %d; want 3 (user + asst 2 tool_use + user 2 tool_result)", len(res.Messages))
	}
	asst := res.Messages[1]
	if got := collectTypeIDs(asst.Content, llm.BlockToolUse); len(got) != 2 || got[0] != "c1" || got[1] != "c2" {
		t.Errorf("asst tool_use IDs = %v; want [c1 c2]", got)
	}
	user := res.Messages[2]
	if got := collectTypeIDs(user.Content, llm.BlockToolResult); len(got) != 2 || got[0] != "c1" || got[1] != "c2" {
		t.Errorf("user tool_result IDs = %v; want [c1 c2] (same order)", got)
	}
}

// ============================================================
// Unknown-tool guard: no orphan tool_use in committed history
// ============================================================

// TestIntegration_UnknownTool_NoOrphanTooluse exercises the orphan-
// prevention contract through fake.Provider: assistant requests an
// unregistered tool → DIAGNOSE.TOOL_UNKNOWN terminal, and the
// committed transcript MUST NOT contain a dangling tool_use (which
// would 400 on any subsequent provider call).
func TestIntegration_UnknownTool_NoOrphanTooluse(t *testing.T) {
	t.Parallel()
	reg, _ := NewRegistry(ClockTool{})
	prov := fake.NewScriptedTurns(fake.Turn{
		ToolUses: []llm.ToolUse{{ID: "ghost", Name: "not-registered"}},
		Finish:   llm.FinishToolUse,
	})
	loop, _ := NewLoop(Options{Provider: prov, Registry: reg})
	res, err := loop.Run(context.Background(), userReq("?"), (&recorder{}).emit)
	if !errors.Is(err, ErrToolUnknown) {
		t.Errorf("Run err = %v; want ErrToolUnknown", err)
	}
	// The only message in the committed transcript is the caller's input.
	if len(res.Messages) != 1 {
		t.Errorf("Messages = %d; want 1 (no assistant turn — orphan tool_use prevented)", len(res.Messages))
	}
	assertPairedHistory(t, res.Messages)
}

// ============================================================
// fake.Provider script-exhaustion → REQUEST_INVALID (script overshoot)
// ============================================================

// TestIntegration_ScriptExhausted_SurfacesAsProviderErr verifies that
// when a test under-scripts the provider (loop wants another turn but
// the script ran out), fake returns LLM.REQUEST_INVALID instead of
// silently re-running the last turn. The loop then surfaces this as
// a FinishError terminal — exactly what we want from a noisy test.
func TestIntegration_ScriptExhausted_SurfacesAsProviderErr(t *testing.T) {
	t.Parallel()
	reg, _ := NewRegistry(ClockTool{Now: fixedClockNow})
	// Script ONE turn — but the loop will need a second one (to consume
	// the tool_result after the tool runs).
	prov := fake.NewScriptedTurns(fake.Turn{
		ToolUses: []llm.ToolUse{{ID: "c1", Name: "clock"}},
		Finish:   llm.FinishToolUse,
	})
	loop, _ := NewLoop(Options{Provider: prov, Registry: reg})
	_, err := loop.Run(context.Background(), userReq("when"), (&recorder{}).emit)
	if !errors.Is(err, llm.ErrRequestInvalid) {
		t.Errorf("Run err = %v; want LLM.REQUEST_INVALID (script overshoot)", err)
	}
	if prov.CallCount() != 2 {
		t.Errorf("provider call count = %d; want 2 (second call triggered the overshoot)", prov.CallCount())
	}
}
