// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package skillinvoke_test — spec-2.3 D-7 integration: the REAL
// invoke.SkillTool wired into the REAL diagnose.Loop with a scripted
// fake provider, exercised end-to-end against the CC golden skill
// (code-reviewer.md, spec-2.1 D-7 corpus):
//
//	invoke → body in transcript → scope narrows → filtered tool denied
//	(paired) → Skill implicitly retained → second skill replaces scope
//	→ now-allowed tool executes → dedup cached replay keeps the scope.
//
// This suite is also the behavioral drift-guard for the wire name: the
// Loop's implicit-retention constant and invoke.ToolName must agree, or
// the "second skill after restrictive scope" step below fails.
package skillinvoke_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/sqlrush/opendbx/internal/app/diagnose"
	"github.com/sqlrush/opendbx/internal/app/skills"
	"github.com/sqlrush/opendbx/internal/app/skills/invoke"
	"github.com/sqlrush/opendbx/internal/domain/llm"
	"github.com/sqlrush/opendbx/internal/domain/llm/fake"
)

// loadGolden parses the spec-2.1 CC golden skill (allowed-tools: Read,
// Grep, Glob, Bash — none registered in opendbx, and no "Skill" entry).
func loadGolden(t *testing.T) skills.Skill {
	t.Helper()
	raw, err := os.ReadFile("../../../internal/app/skills/testdata/code-reviewer.md")
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	sk, err := skills.Parse(raw, skills.SkillSource{Kind: skills.SourceProject, Precedence: 400})
	if err != nil {
		t.Fatalf("parse golden: %v", err)
	}
	return sk
}

// dbHelper is a synthetic second skill whose scope allows echo.
func dbHelper() skills.Skill {
	return skills.Skill{
		Schema: skills.Schema{
			Name:         "db-helper",
			Description:  "Echo-driven helper for integration tests.",
			AllowedTools: "echo",
		},
		Body: "Use the echo tool.",
	}
}

// newLoop builds a production-shaped Loop: clock + echo + the real
// SkillTool over the given active set.
func newLoop(t *testing.T, prov llm.Provider, active []skills.Skill, dedup bool) *diagnose.Loop {
	t.Helper()
	st, err := invoke.NewSkillTool(active)
	if err != nil {
		t.Fatalf("NewSkillTool: %v", err)
	}
	reg, err := diagnose.NewRegistry(diagnose.ClockTool{}, diagnose.EchoTool{}, st)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	loop, err := diagnose.NewLoop(diagnose.Options{
		Provider: prov, Registry: reg, MaxTurns: 10, DedupEnabled: dedup,
	})
	if err != nil {
		t.Fatalf("NewLoop: %v", err)
	}
	return loop
}

func userReq(text string) llm.Request {
	return llm.Request{
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: []llm.ContentBlock{{Type: llm.BlockText, Text: text}}},
		},
		MaxTokens: 1024,
	}
}

// toolResults flattens every tool_result block in transcript order.
func toolResults(msgs []llm.Message) []llm.ToolResult {
	var out []llm.ToolResult
	for _, m := range msgs {
		for _, c := range m.Content {
			if c.Type == llm.BlockToolResult && c.ToolResult != nil {
				out = append(out, *c.ToolResult)
			}
		}
	}
	return out
}

// TestSkillInvoke_FullChain — the spec-2.3 D-7 scripted journey.
func TestSkillInvoke_FullChain(t *testing.T) {
	t.Parallel()
	golden := loadGolden(t)
	prov := fake.NewScriptedTurns(
		// turn1: enter the golden skill scope {Read, Grep, Glob, Bash}.
		fake.Turn{Finish: llm.FinishToolUse, ToolUses: []llm.ToolUse{
			{ID: "c1", Name: "Skill", Input: map[string]any{"skill": "code-reviewer"}}}},
		// turn2: echo is registered but not in scope → denied (recoverable).
		fake.Turn{Finish: llm.FinishToolUse, ToolUses: []llm.ToolUse{
			{ID: "c2", Name: "echo", Input: map[string]any{"k": "v"}}}},
		// turn3: "Skill" is implicitly retained → switching skills works
		// even though the golden scope does not list it (Q13).
		fake.Turn{Finish: llm.FinishToolUse, ToolUses: []llm.ToolUse{
			{ID: "c3", Name: "Skill", Input: map[string]any{"skill": "db-helper"}}}},
		// turn4: echo now in scope → executes for real.
		fake.Turn{Finish: llm.FinishToolUse, ToolUses: []llm.ToolUse{
			{ID: "c4", Name: "echo", Input: map[string]any{"k": "v"}}}},
		fake.Turn{Text: "done", Finish: llm.FinishStop},
	)
	loop := newLoop(t, prov, []skills.Skill{golden, dbHelper()}, false)

	calls, results := map[string]bool{}, map[string]bool{}
	emit := func(_ context.Context, e diagnose.Event) error {
		switch e.Kind {
		case diagnose.EventToolCall:
			calls[e.ToolUse.ID] = true
		case diagnose.EventToolResult:
			results[e.ToolResult.ToolUseID] = true
		}
		return nil
	}
	res, err := loop.Run(context.Background(), userReq("review my code"), emit)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.FinishReason != llm.FinishStop || res.Turns != 5 {
		t.Fatalf("Result = %+v; want FinishStop/5", res)
	}

	// UI pairing: every EventToolCall got its EventToolResult (user
	// regression mandate — nothing stuck Running).
	for id := range calls {
		if !results[id] {
			t.Errorf("tool call %s has no EventToolResult (stuck Running)", id)
		}
	}

	trs := toolResults(res.Messages)
	if len(trs) != 4 {
		t.Fatalf("tool_result blocks = %d; want 4 (paired commit)", len(trs))
	}
	// c1: golden body byte-verbatim + Q7 model notice (model: sonnet).
	if !strings.HasPrefix(trs[0].Content, golden.Body) || trs[0].IsError {
		t.Errorf("c1 = %+v; want golden body prefix", trs[0])
	}
	if !strings.Contains(trs[0].Content, `requests model "sonnet"`) {
		t.Errorf("c1 missing Q7 model notice: %q", trs[0].Content)
	}
	// c2: denied with the SCOPE_TOOL_DENIED template listing the scope.
	if !trs[1].IsError || !strings.Contains(trs[1].Content, "SKILL.SCOPE_TOOL_DENIED") ||
		!strings.Contains(trs[1].Content, "Allowed: [Read, Grep, Glob, Bash] (+ Skill)") {
		t.Errorf("c2 = %+v; want SCOPE_TOOL_DENIED with scope listing", trs[1])
	}
	// c3: second skill invoke succeeded (implicit Skill retention).
	if trs[2].IsError || !strings.HasPrefix(trs[2].Content, "Use the echo tool.") {
		t.Errorf("c3 = %+v; want db-helper body (Skill retained, Q13)", trs[2])
	}
	// c4: echo executed for real under the replaced scope.
	if trs[3].IsError || trs[3].Content != `{"k":"v"}` {
		t.Errorf("c4 = %+v; want real echo output under widened scope", trs[3])
	}
}

// TestSkillInvoke_CachedReplayKeepsScope — invoking the same skill with
// the same input within the dedup window replays the cached body AND its
// ToolFilter (scope update is never gated on !cached; spec-2.3 DoD).
func TestSkillInvoke_CachedReplayKeepsScope(t *testing.T) {
	t.Parallel()
	prov := fake.NewScriptedTurns(
		fake.Turn{Finish: llm.FinishToolUse, ToolUses: []llm.ToolUse{
			{ID: "c1", Name: "Skill", Input: map[string]any{"skill": "db-helper"}}}},
		// Same name+input → dedup cached hit; ToolFilter {echo} replayed.
		fake.Turn{Finish: llm.FinishToolUse, ToolUses: []llm.ToolUse{
			{ID: "c2", Name: "Skill", Input: map[string]any{"skill": "db-helper"}}}},
		// clock is registered but outside {echo} scope → still denied.
		fake.Turn{Finish: llm.FinishToolUse, ToolUses: []llm.ToolUse{
			{ID: "c3", Name: "clock", Input: map[string]any{}}}},
		fake.Turn{Text: "done", Finish: llm.FinishStop},
	)
	loop := newLoop(t, prov, []skills.Skill{dbHelper()}, true)

	var c2Cached bool
	emit := func(_ context.Context, e diagnose.Event) error {
		if e.Kind == diagnose.EventToolResult && e.ToolResult.ToolUseID == "c2" {
			c2Cached = e.Cached
		}
		return nil
	}
	res, err := loop.Run(context.Background(), userReq("go"), emit)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !c2Cached {
		t.Fatal("c2 expected to be a dedup cached hit")
	}
	trs := toolResults(res.Messages)
	// Cached content byte-identical to the fresh run (§ 3.6 wire-clean).
	if trs[0].Content != trs[1].Content {
		t.Errorf("cached content differs from fresh:\n%q\nvs\n%q", trs[0].Content, trs[1].Content)
	}
	// Scope still live after the cached replay: clock denied.
	if !trs[2].IsError || !strings.Contains(trs[2].Content, "SKILL.SCOPE_TOOL_DENIED") {
		t.Errorf("c3 = %+v; want denial (cached replay kept scope)", trs[2])
	}
}
