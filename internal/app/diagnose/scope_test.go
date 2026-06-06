// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File scope_test.go — spec-2.3 D-3 allowed-tools scope enforcement.
//
// User-mandated regression (approve note 2026-06-05): a filtered tool
// call must (a) never leave the UI stuck Running — its EventToolCall is
// always paired with an EventToolResult — and (b) keep the transcript
// paired-commit shape intact (every tool_use has a tool_result).

package diagnose

import (
	"context"
	"strings"
	"testing"

	"github.com/sqlrush/opendbx/internal/domain/llm"
)

// scopeTool is a scripted ToolExecutor that returns a ToolFilter —
// stands in for the spec-2.3 SkillTool without importing app/skills
// (invoke imports diagnose; the reverse would cycle).
type scopeTool struct {
	name   string
	filter []string
	isErr  bool
}

func (s scopeTool) Name() string { return s.name }
func (s scopeTool) Schema() llm.ToolSchema {
	return llm.ToolSchema{Name: s.name, InputSchema: map[string]any{"type": "object"}}
}
func (s scopeTool) Execute(_ context.Context, _ map[string]any) (ToolOutput, error) {
	return ToolOutput{Content: "scope:" + s.name, IsError: s.isErr, ToolFilter: s.filter}, nil
}

// capProv wraps stubProv and records every Request the Loop sends, so
// tests can assert the advertised (filtered) tool set per turn.
type capProv struct {
	stubProv
	reqs []llm.Request
}

func (p *capProv) Stream(ctx context.Context, req llm.Request) (llm.Stream, error) {
	p.reqs = append(p.reqs, req)
	return p.stubProv.Stream(ctx, req)
}

// toolNames extracts the advertised tool names of a captured request.
func toolNames(req llm.Request) []string {
	out := make([]string, 0, len(req.Tools))
	for _, ts := range req.Tools {
		out = append(out, ts.Name)
	}
	return out
}

// toolUseChunk scripts a FinishToolUse turn with the given calls.
func toolUseChunk(uses ...llm.ToolUse) llm.Chunk {
	return llm.Chunk{FinishReason: llm.FinishToolUse, ToolUses: uses}
}

// stopChunk scripts a natural FinishStop turn.
func stopChunk() llm.Chunk { return llm.Chunk{Token: "done", FinishReason: llm.FinishStop} }

// ============================================================
// applyFilter / normalizeFilter units
// ============================================================

func TestApplyFilter_NilKeepsIdentity(t *testing.T) {
	t.Parallel()
	tools := []llm.ToolSchema{{Name: "clock"}, {Name: "echo"}}
	got := applyFilter(tools, nil)
	// Slice-header identity: same length/cap AND same backing array
	// (element-0 address), guarded by the len check first (go LOW-3).
	if len(got) != len(tools) || cap(got) != cap(tools) {
		t.Fatalf("nil filter changed slice shape: len/cap %d/%d vs %d/%d",
			len(got), cap(got), len(tools), cap(tools))
	}
	if len(tools) > 0 && &got[0] != &tools[0] {
		t.Error("nil filter must return the original slice (identity; prompt-cache stability)")
	}
}

func TestApplyFilter_NarrowsAndRetainsSkill(t *testing.T) {
	t.Parallel()
	tools := []llm.ToolSchema{{Name: "clock"}, {Name: "echo"}, {Name: skillToolName}}
	got := applyFilter(tools, []string{"clock"})
	if len(got) != 2 || got[0].Name != "clock" || got[1].Name != skillToolName {
		t.Errorf("applyFilter = %v; want [clock %s] (Skill implicitly retained, Q13)", got, skillToolName)
	}
	// Original slice untouched (Rule 12 immutability).
	if len(tools) != 3 {
		t.Error("applyFilter mutated its input")
	}
	// Non-nil empty filter → only Skill survives.
	if got := applyFilter(tools, []string{}); len(got) != 1 || got[0].Name != skillToolName {
		t.Errorf("empty filter = %v; want [%s] only", got, skillToolName)
	}
}

func TestNormalizeFilter(t *testing.T) {
	t.Parallel()
	if normalizeFilter(nil) != nil {
		t.Error("nil must stay nil (inherit)")
	}
	got := normalizeFilter([]string{"b", "a", "b", "A"})
	want := []string{"b", "a", "A"} // case-sensitive dedupe, first occurrence order
	if len(got) != len(want) {
		t.Fatalf("normalizeFilter = %v; want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("normalizeFilter[%d] = %q; want %q", i, got[i], want[i])
		}
	}
	if e := normalizeFilter([]string{}); e == nil || len(e) != 0 {
		t.Error("non-nil empty must stay non-nil empty (empty execution scope)")
	}
}

// ============================================================
// Dispatch guard — the user-mandated regression
// ============================================================

// TestRun_ScopeDenied_PairedAndNotStuck — turn1 enters a scope allowing
// only clock; turn2 the model calls echo (registered but filtered). The
// denied call must emit BOTH EventToolCall and EventToolResult (UI never
// stuck Running), append an IsError tool_result to the transcript
// (paired commit intact), and the Run continues — not terminal.
func TestRun_ScopeDenied_PairedAndNotStuck(t *testing.T) {
	t.Parallel()
	reg, _ := NewRegistry(scopeTool{name: skillToolName, filter: []string{"clock"}}, EchoTool{}, ClockTool{})
	prov := &capProv{stubProv: stubProv{turns: []stubTurn{
		{chunks: []llm.Chunk{toolUseChunk(llm.ToolUse{ID: "c1", Name: skillToolName})}},
		{chunks: []llm.Chunk{toolUseChunk(llm.ToolUse{ID: "c2", Name: "echo"})}},
		{chunks: []llm.Chunk{stopChunk()}},
	}}}
	r := &recorder{}
	res, err := mustNewLoop(t, Options{Provider: prov, Registry: reg}).
		Run(context.Background(), userReq("go"), r.emit)
	if err != nil {
		t.Fatalf("denied call must not terminate the Run: %v", err)
	}
	if res.FinishReason != llm.FinishStop || res.Turns != 3 {
		t.Fatalf("Result = %+v; want FinishStop/3", res)
	}

	// (a) UI pairing: every EventToolCall has a matching EventToolResult.
	calls, results := 0, 0
	var deniedResult *llm.ToolResult
	for _, e := range r.events {
		switch e.Kind {
		case EventToolCall:
			calls++
		case EventToolResult:
			results++
			if e.ToolResult.ToolUseID == "c2" {
				deniedResult = e.ToolResult
			}
		}
	}
	if calls != 2 || results != 2 {
		t.Errorf("calls/results = %d/%d; want 2/2 (denied tool must not be stuck Running)", calls, results)
	}
	if deniedResult == nil {
		t.Fatal("no EventToolResult for the denied call c2")
	}
	if !deniedResult.IsError || !strings.Contains(deniedResult.Content, "SKILL.SCOPE_TOOL_DENIED") {
		t.Errorf("denied result = %+v; want IsError + SCOPE_TOOL_DENIED template", deniedResult)
	}
	if !strings.Contains(deniedResult.Content, "Allowed: [clock]") ||
		!strings.Contains(deniedResult.Content, "(+ Skill)") {
		t.Errorf("denied content must list the active scope: %q", deniedResult.Content)
	}

	// (b) Transcript pairing: msgs = user, asst(c1), user(result c1), asst(c2), user(result c2).
	if len(res.Messages) != 5 {
		t.Fatalf("Messages = %d; want 5 (paired commit per turn)", len(res.Messages))
	}
	denied := res.Messages[4].Content[0].ToolResult
	if denied.ToolUseID != "c2" || !denied.IsError {
		t.Errorf("transcript denied result = %+v; want c2/IsError", denied)
	}

	// (c) Advertise narrowing: turn2/turn3 requests offer clock + Skill only.
	for i := 1; i < 3; i++ {
		names := toolNames(prov.reqs[i])
		if len(names) != 2 || names[0] != skillToolName && names[1] != skillToolName {
			t.Errorf("turn %d advertised %v; want [clock %s] in registry order", i+1, names, skillToolName)
		}
		for _, n := range names {
			if n != "clock" && n != skillToolName {
				t.Errorf("turn %d advertised filtered-out tool %q", i+1, n)
			}
		}
	}
}

// TestRun_ScopeIntraTurnImmediate — one assistant turn emits
// [Skill, echo]; the scope set by Skill (executed first, Phase 2 serial)
// applies to echo IN THE SAME TURN (Q12 user decision: immediate effect).
func TestRun_ScopeIntraTurnImmediate(t *testing.T) {
	t.Parallel()
	reg, _ := NewRegistry(scopeTool{name: skillToolName, filter: []string{"clock"}}, EchoTool{}, ClockTool{})
	prov := &capProv{stubProv: stubProv{turns: []stubTurn{
		{chunks: []llm.Chunk{toolUseChunk(
			llm.ToolUse{ID: "c1", Name: skillToolName},
			llm.ToolUse{ID: "c2", Name: "echo"},
		)}},
		{chunks: []llm.Chunk{stopChunk()}},
	}}}
	r := &recorder{}
	res, err := mustNewLoop(t, Options{Provider: prov, Registry: reg}).
		Run(context.Background(), userReq("go"), r.emit)
	if err != nil {
		t.Fatalf("Run err: %v", err)
	}
	// Transcript: user, asst(c1+c2), user(result c1 + result c2).
	if len(res.Messages) != 3 {
		t.Fatalf("Messages = %d; want 3", len(res.Messages))
	}
	resultsMsg := res.Messages[2]
	if len(resultsMsg.Content) != 2 {
		t.Fatalf("tool_result blocks = %d; want 2 (paired)", len(resultsMsg.Content))
	}
	second := resultsMsg.Content[1].ToolResult
	if second.ToolUseID != "c2" || !second.IsError ||
		!strings.Contains(second.Content, "SKILL.SCOPE_TOOL_DENIED") {
		t.Errorf("same-turn echo after Skill must be denied (immediate effect, Q12): %+v", second)
	}
}

// TestRun_ScopeReplaceAndInherit — a nil ToolFilter inherits the current
// scope; a non-nil one REPLACES it. After replacing with a scope that
// allows echo, echo executes for real (and a previously denied call is
// never served from dedup — denied calls skip the cache entirely).
func TestRun_ScopeReplaceAndInherit(t *testing.T) {
	t.Parallel()
	reg, _ := NewRegistry(
		scopeTool{name: skillToolName, filter: []string{"clock"}},
		scopeTool{name: "inherit", filter: nil}, // registered execution tool returning nil filter
		scopeTool{name: "widen", filter: []string{"echo", "inherit"}},
		EchoTool{}, ClockTool{},
	)
	prov := &capProv{stubProv: stubProv{turns: []stubTurn{
		// turn1: enter scope {clock} — note: "inherit"/"widen" become filtered.
		{chunks: []llm.Chunk{toolUseChunk(llm.ToolUse{ID: "c1", Name: skillToolName})}},
		// turn2: echo denied under {clock}.
		{chunks: []llm.Chunk{toolUseChunk(llm.ToolUse{ID: "c2", Name: "echo"})}},
		// turn3: Skill itself is retained; its (scripted) filter replaces scope with {clock} again —
		//        then turn4 calls clock: allowed, and clock's nil ToolFilter must NOT clear scope.
		{chunks: []llm.Chunk{toolUseChunk(llm.ToolUse{ID: "c3", Name: skillToolName})}},
		{chunks: []llm.Chunk{toolUseChunk(llm.ToolUse{ID: "c4", Name: "clock"})}},
		// turn5: echo still denied (nil filter from clock inherited the {clock} scope).
		{chunks: []llm.Chunk{toolUseChunk(llm.ToolUse{ID: "c5", Name: "echo", Input: map[string]any{"k": "v"}})}},
		{chunks: []llm.Chunk{stopChunk()}},
	}}}
	r := &recorder{}
	res, err := mustNewLoop(t, Options{Provider: prov, Registry: reg}).
		Run(context.Background(), userReq("go"), r.emit)
	if err != nil {
		t.Fatalf("Run err: %v", err)
	}
	if res.Turns != 6 {
		t.Fatalf("Turns = %d; want 6", res.Turns)
	}
	// c4 (clock) executed for real under scope.
	clockResult := res.Messages[8].Content[0].ToolResult
	if clockResult.ToolUseID != "c4" || clockResult.IsError {
		t.Errorf("clock under scope must execute: %+v", clockResult)
	}
	// c5 (echo) still denied — clock's nil ToolFilter did not clear the scope.
	echoResult := res.Messages[10].Content[0].ToolResult
	if echoResult.ToolUseID != "c5" || !echoResult.IsError ||
		!strings.Contains(echoResult.Content, "SKILL.SCOPE_TOOL_DENIED") {
		t.Errorf("nil ToolFilter must inherit (not clear) the scope: %+v", echoResult)
	}
}

// TestRun_ScopeIsErrorNoUpdate — a ToolOutput carrying IsError must not
// update the scope even when ToolFilter is non-nil.
func TestRun_ScopeIsErrorNoUpdate(t *testing.T) {
	t.Parallel()
	reg, _ := NewRegistry(
		scopeTool{name: skillToolName, filter: []string{"clock"}, isErr: true},
		EchoTool{}, ClockTool{},
	)
	prov := &capProv{stubProv: stubProv{turns: []stubTurn{
		{chunks: []llm.Chunk{toolUseChunk(llm.ToolUse{ID: "c1", Name: skillToolName})}},
		{chunks: []llm.Chunk{toolUseChunk(llm.ToolUse{ID: "c2", Name: "echo"})}},
		{chunks: []llm.Chunk{stopChunk()}},
	}}}
	r := &recorder{}
	res, err := mustNewLoop(t, Options{Provider: prov, Registry: reg}).
		Run(context.Background(), userReq("go"), r.emit)
	if err != nil {
		t.Fatalf("Run err: %v", err)
	}
	echoResult := res.Messages[4].Content[0].ToolResult
	if echoResult.IsError {
		t.Errorf("IsError output must not enter scope; echo should run free: %+v", echoResult)
	}
	if names := toolNames(prov.reqs[1]); len(names) != 3 {
		t.Errorf("turn2 advertised %v; want all 3 (no scope from IsError)", names)
	}
}

// TestRun_ScopeFromCachedHit — a dedup cached hit must replay its
// ToolFilter (the scope update is gated on success only, NEVER on
// !cached; spec-2.3 DoD negative assertion).
func TestRun_ScopeFromCachedHit(t *testing.T) {
	t.Parallel()
	reg, _ := NewRegistry(
		scopeTool{name: skillToolName, filter: []string{"clock"}},
		scopeTool{name: "widen", filter: []string{"echo", "widen", "clock"}},
		EchoTool{}, ClockTool{},
	)
	prov := &capProv{stubProv: stubProv{turns: []stubTurn{
		// turn1: Skill{} → scope {clock} (stored in dedup under its input hash).
		{chunks: []llm.Chunk{toolUseChunk(llm.ToolUse{ID: "c1", Name: skillToolName})}},
		// turn2: Skill is retained → widen is NOT in scope... use Skill replay instead:
		// call Skill again with the SAME (empty) input → cached hit → scope re-applied.
		{chunks: []llm.Chunk{toolUseChunk(llm.ToolUse{ID: "c2", Name: skillToolName})}},
		// turn3: echo still denied — the cached replay kept the {clock} scope live.
		{chunks: []llm.Chunk{toolUseChunk(llm.ToolUse{ID: "c3", Name: "echo"})}},
		{chunks: []llm.Chunk{stopChunk()}},
	}}}
	r := &recorder{}
	res, err := mustNewLoop(t, Options{Provider: prov, Registry: reg, DedupEnabled: true}).
		Run(context.Background(), userReq("go"), r.emit)
	if err != nil {
		t.Fatalf("Run err: %v", err)
	}
	// The c2 result must be the cached replay (Cached flag on the event).
	var c2Cached bool
	for _, e := range r.events {
		if e.Kind == EventToolResult && e.ToolResult.ToolUseID == "c2" {
			c2Cached = e.Cached
		}
	}
	if !c2Cached {
		t.Fatal("c2 expected to be a dedup cached hit (same name+input within window)")
	}
	echoResult := res.Messages[6].Content[0].ToolResult
	if !echoResult.IsError || !strings.Contains(echoResult.Content, "SKILL.SCOPE_TOOL_DENIED") {
		t.Errorf("cached hit must replay ToolFilter (no !cached gate): %+v", echoResult)
	}
}

// TestRun_UnknownToolStillTerminal — regression: the scope guard must not
// soften the pre-existing unknown-tool terminal (loop.go FinishToolUse
// validation; spec-1.21 D-4).
func TestRun_UnknownToolStillTerminal(t *testing.T) {
	t.Parallel()
	reg, _ := NewRegistry(scopeTool{name: skillToolName, filter: []string{"clock"}}, ClockTool{})
	prov := &capProv{stubProv: stubProv{turns: []stubTurn{
		{chunks: []llm.Chunk{toolUseChunk(llm.ToolUse{ID: "c1", Name: skillToolName})}},
		// "ghost" is not registered at all — terminal, even though a scope is active.
		{chunks: []llm.Chunk{toolUseChunk(llm.ToolUse{ID: "c2", Name: "ghost"})}},
	}}}
	r := &recorder{}
	res, err := mustNewLoop(t, Options{Provider: prov, Registry: reg}).
		Run(context.Background(), userReq("go"), r.emit)
	if err == nil {
		t.Fatal("unknown tool must stay terminal")
	}
	if res.TermCode != ErrToolUnknown.Code() {
		t.Errorf("TermCode = %q; want %q", res.TermCode, ErrToolUnknown.Code())
	}
}
