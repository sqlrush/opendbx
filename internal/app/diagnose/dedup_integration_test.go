// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File dedup_integration_test.go — spec-1.22 D-2/D-7: per-Run dedup cache
// behavior driven end-to-end through diagnose.Loop + fake scripted turns.

package diagnose

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sqlrush/opendbx/internal/domain/llm"
	"github.com/sqlrush/opendbx/internal/domain/llm/fake"
)

// spyTool counts Execute invocations and returns a fixed content. It does NOT
// implement CacheableTool, so it defaults to cacheable.
type spyTool struct {
	name  string
	calls *int
	out   string
}

func (s spyTool) Name() string { return s.name }

func (s spyTool) Schema() llm.ToolSchema {
	return llm.ToolSchema{
		Name:        s.name,
		Description: "spy",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
	}
}

func (s spyTool) Execute(_ context.Context, _ map[string]any) (ToolOutput, error) {
	*s.calls++
	return ToolOutput{Content: s.out}, nil
}

// optOutSpyTool is a spyTool that opts out of dedup (Cacheable() == false).
type optOutSpyTool struct{ spyTool }

func (optOutSpyTool) Cacheable() bool { return false }

// spyToolSchema builds the no-input schema shared by the test spy tools.
func spyToolSchema(name, desc string) llm.ToolSchema {
	return llm.ToolSchema{
		Name:        name,
		Description: desc,
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
	}
}

// budgetBurnTool blocks until its ctx deadline fires, then returns SUCCESS
// (ignoring the ctx error). This deterministically expires the total budget
// before returning a storable success — driving the cache-hit totalCtx guard
// WITHOUT a flaky wall-clock margin (go-review LOW-1): it waits for the actual
// deadline rather than racing a fixed sleep against it.
type budgetBurnTool struct {
	name  string
	calls *int
}

func (s budgetBurnTool) Name() string           { return s.name }
func (s budgetBurnTool) Schema() llm.ToolSchema { return spyToolSchema(s.name, "budget burn") }
func (s budgetBurnTool) Execute(ctx context.Context, _ map[string]any) (ToolOutput, error) {
	*s.calls++
	<-ctx.Done() // wait for the (total) deadline to fire, then succeed anyway
	return ToolOutput{Content: "burned-ok"}, nil
}

// errorTool returns a Go error from Execute (infrastructure-failure shape):
// the ToolOutput is the zero value, so out.IsError==false. Used to pin the
// store guard execErr==nil (a `!out.IsError`-only guard would wrongly cache it).
type errorTool struct {
	name  string
	calls *int
}

func (s errorTool) Name() string           { return s.name }
func (s errorTool) Schema() llm.ToolSchema { return spyToolSchema(s.name, "always errors") }
func (s errorTool) Execute(_ context.Context, _ map[string]any) (ToolOutput, error) {
	*s.calls++
	return ToolOutput{}, errors.New("boom")
}

// timeoutTool respects ctx and returns its deadline error — the per-tool
// timeout shape (recoverable feedback, IsError=true, NOT terminal while the
// total budget remains). Also a zero ToolOutput, so it pins the same guard.
type timeoutTool struct {
	name  string
	calls *int
}

func (s timeoutTool) Name() string           { return s.name }
func (s timeoutTool) Schema() llm.ToolSchema { return spyToolSchema(s.name, "always times out") }
func (s timeoutTool) Execute(ctx context.Context, _ map[string]any) (ToolOutput, error) {
	*s.calls++
	<-ctx.Done()
	return ToolOutput{}, ctx.Err()
}

func toolResultEvents(events []Event) []Event {
	var out []Event
	for _, e := range events {
		if e.Kind == EventToolResult {
			out = append(out, e)
		}
	}
	return out
}

// TestDedup_SameParams_SecondTurnCached — an identical (name, Input) call on a
// later turn is served from cache: Execute runs once, the second EventToolResult
// is marked Cached, and BOTH tool_result wire contents stay byte-identical
// (clean result, no marker — invariant #2 / § 3.6 errata).
func TestDedup_SameParams_SecondTurnCached(t *testing.T) {
	t.Parallel()
	calls := 0
	reg, err := NewRegistry(spyTool{name: "spy", calls: &calls, out: "RESULT"})
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	in := map[string]any{"v": "a"}
	prov := fake.NewScriptedTurns(
		fake.Turn{ToolUses: []llm.ToolUse{{ID: "t1", Name: "spy", Input: in}}, Finish: llm.FinishToolUse},
		fake.Turn{ToolUses: []llm.ToolUse{{ID: "t2", Name: "spy", Input: in}}, Finish: llm.FinishToolUse},
		fake.Turn{Text: "done", Finish: llm.FinishStop},
	)
	loop := mustNewLoop(t, Options{Provider: prov, Registry: reg, DedupEnabled: true})
	r := &recorder{}
	res, runErr := loop.Run(context.Background(), userReq("q"), r.emit)
	if runErr != nil {
		t.Fatalf("Run: %v", runErr)
	}
	if calls != 1 {
		t.Errorf("Execute calls = %d; want 1 (2nd identical call served from cache)", calls)
	}
	trs := toolResultEvents(r.events)
	if len(trs) != 2 {
		t.Fatalf("EventToolResult count = %d; want 2", len(trs))
	}
	if trs[0].Cached {
		t.Error("first call must NOT be Cached")
	}
	if !trs[1].Cached {
		t.Error("second identical call MUST be Cached")
	}
	for i, m := range res.Messages {
		for _, b := range m.Content {
			if b.Type == llm.BlockToolResult && b.ToolResult.Content != "RESULT" {
				t.Errorf("msg %d tool_result content = %q; want clean \"RESULT\" (no marker, invariant #2)", i, b.ToolResult.Content)
			}
		}
	}
}

func TestDedup_DifferentParams_NotCached(t *testing.T) {
	t.Parallel()
	calls := 0
	reg, _ := NewRegistry(spyTool{name: "spy", calls: &calls, out: "R"})
	prov := fake.NewScriptedTurns(
		fake.Turn{ToolUses: []llm.ToolUse{{ID: "t1", Name: "spy", Input: map[string]any{"v": "a"}}}, Finish: llm.FinishToolUse},
		fake.Turn{ToolUses: []llm.ToolUse{{ID: "t2", Name: "spy", Input: map[string]any{"v": "b"}}}, Finish: llm.FinishToolUse},
		fake.Turn{Text: "done", Finish: llm.FinishStop},
	)
	loop := mustNewLoop(t, Options{Provider: prov, Registry: reg, DedupEnabled: true})
	r := &recorder{}
	if _, err := loop.Run(context.Background(), userReq("q"), r.emit); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if calls != 2 {
		t.Errorf("Execute calls = %d; want 2 (different params must not cache)", calls)
	}
}

// TestDedup_Disabled_AlwaysExecutes — with DedupEnabled false (the zero value,
// = spec-1.21 behavior) every call runs and nothing is ever marked Cached.
func TestDedup_Disabled_AlwaysExecutes(t *testing.T) {
	t.Parallel()
	calls := 0
	reg, _ := NewRegistry(spyTool{name: "spy", calls: &calls, out: "R"})
	in := map[string]any{"v": "a"}
	prov := fake.NewScriptedTurns(
		fake.Turn{ToolUses: []llm.ToolUse{{ID: "t1", Name: "spy", Input: in}}, Finish: llm.FinishToolUse},
		fake.Turn{ToolUses: []llm.ToolUse{{ID: "t2", Name: "spy", Input: in}}, Finish: llm.FinishToolUse},
		fake.Turn{Text: "done", Finish: llm.FinishStop},
	)
	loop := mustNewLoop(t, Options{Provider: prov, Registry: reg}) // DedupEnabled omitted = false
	r := &recorder{}
	if _, err := loop.Run(context.Background(), userReq("q"), r.emit); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if calls != 2 {
		t.Errorf("Execute calls = %d; want 2 (dedup disabled)", calls)
	}
	for _, e := range toolResultEvents(r.events) {
		if e.Cached {
			t.Error("disabled dedup must never mark Cached")
		}
	}
}

// TestDedup_WindowExpiry_ReExecutes — window 1: stored at turn 1, the turn-2
// lookup is diff==window → expired → re-executes (boundary).
func TestDedup_WindowExpiry_ReExecutes(t *testing.T) {
	t.Parallel()
	calls := 0
	reg, _ := NewRegistry(spyTool{name: "spy", calls: &calls, out: "R"})
	in := map[string]any{"v": "a"}
	prov := fake.NewScriptedTurns(
		fake.Turn{ToolUses: []llm.ToolUse{{ID: "t1", Name: "spy", Input: in}}, Finish: llm.FinishToolUse},
		fake.Turn{ToolUses: []llm.ToolUse{{ID: "t2", Name: "spy", Input: in}}, Finish: llm.FinishToolUse},
		fake.Turn{Text: "done", Finish: llm.FinishStop},
	)
	loop := mustNewLoop(t, Options{Provider: prov, Registry: reg, DedupEnabled: true, DedupWindow: 1})
	r := &recorder{}
	if _, err := loop.Run(context.Background(), userReq("q"), r.emit); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if calls != 2 {
		t.Errorf("Execute calls = %d; want 2 (window 1 → turn-2 lookup expired)", calls)
	}
}

// TestDedup_NonCacheableTool_AlwaysExecutes — Cacheable()==false bypasses the
// cache entirely (no key, no lookup, no store — invariant #5).
func TestDedup_NonCacheableTool_AlwaysExecutes(t *testing.T) {
	t.Parallel()
	calls := 0
	reg, _ := NewRegistry(optOutSpyTool{spyTool{name: "noc", calls: &calls, out: "R"}})
	in := map[string]any{"v": "a"}
	prov := fake.NewScriptedTurns(
		fake.Turn{ToolUses: []llm.ToolUse{{ID: "t1", Name: "noc", Input: in}}, Finish: llm.FinishToolUse},
		fake.Turn{ToolUses: []llm.ToolUse{{ID: "t2", Name: "noc", Input: in}}, Finish: llm.FinishToolUse},
		fake.Turn{Text: "done", Finish: llm.FinishStop},
	)
	loop := mustNewLoop(t, Options{Provider: prov, Registry: reg, DedupEnabled: true})
	r := &recorder{}
	if _, err := loop.Run(context.Background(), userReq("q"), r.emit); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if calls != 2 {
		t.Errorf("Execute calls = %d; want 2 (Cacheable()=false bypasses cache)", calls)
	}
}

// TestDedup_HitRespectsTotalBudget — codex HIGH-3: a cache hit must NOT run
// past the total diagnosis budget. One turn with two identical tool_uses; the
// first deterministically burns the 20ms TotalTimeout (blocks until the
// deadline, then succeeds + stores), then the second is a cache hit whose
// totalCtx guard must terminate with TOTAL_TIMEOUT.
func TestDedup_HitRespectsTotalBudget(t *testing.T) {
	t.Parallel()
	calls := 0
	reg, _ := NewRegistry(budgetBurnTool{name: "burn", calls: &calls})
	in := map[string]any{"v": "a"}
	prov := fake.NewScriptedTurns(
		fake.Turn{ToolUses: []llm.ToolUse{
			{ID: "t1", Name: "burn", Input: in},
			{ID: "t2", Name: "burn", Input: in},
		}, Finish: llm.FinishToolUse},
		fake.Turn{Text: "unreached", Finish: llm.FinishStop},
	)
	loop := mustNewLoop(t, Options{Provider: prov, Registry: reg, DedupEnabled: true, TotalTimeout: 20 * time.Millisecond})
	r := &recorder{}
	_, runErr := loop.Run(context.Background(), userReq("q"), r.emit)
	if !errors.Is(runErr, ErrTotalTimeout) {
		t.Fatalf("Run err = %v; want ErrTotalTimeout (cache hit must not run past total budget)", runErr)
	}
	if calls != 1 {
		t.Errorf("Execute calls = %d; want 1 (1st call ran + stored; 2nd was a cache hit caught by the totalCtx guard)", calls)
	}
}

// TestDedup_ExecError_NotCached — codex post-impl LOW-1 / pre-impl HIGH-1
// regression: an Execute returning (zero ToolOutput, Go error) has
// out.IsError==false, so a store guard of `!out.IsError` ALONE would wrongly
// cache the empty result as a success. The real guard is execErr==nil &&
// !out.IsError, so the second identical call MUST re-execute.
func TestDedup_ExecError_NotCached(t *testing.T) {
	t.Parallel()
	calls := 0
	reg, _ := NewRegistry(errorTool{name: "err", calls: &calls})
	in := map[string]any{"v": "a"}
	prov := fake.NewScriptedTurns(
		fake.Turn{ToolUses: []llm.ToolUse{{ID: "t1", Name: "err", Input: in}}, Finish: llm.FinishToolUse},
		fake.Turn{ToolUses: []llm.ToolUse{{ID: "t2", Name: "err", Input: in}}, Finish: llm.FinishToolUse},
		fake.Turn{Text: "done", Finish: llm.FinishStop},
	)
	loop := mustNewLoop(t, Options{Provider: prov, Registry: reg, DedupEnabled: true})
	r := &recorder{}
	if _, err := loop.Run(context.Background(), userReq("q"), r.emit); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if calls != 2 {
		t.Errorf("Execute calls = %d; want 2 (a Go-error result must NOT be cached as a success)", calls)
	}
	for _, e := range toolResultEvents(r.events) {
		if e.Cached {
			t.Error("an errored call must never be Cached")
		}
	}
}

// TestDedup_ToolTimeout_NotCached — same store-guard regression via the
// per-tool-timeout path: a tool that hits its ToolTimeout returns IsError=true
// feedback + zero ToolOutput + a DeadlineExceeded execErr; it must not cache.
func TestDedup_ToolTimeout_NotCached(t *testing.T) {
	t.Parallel()
	calls := 0
	reg, _ := NewRegistry(timeoutTool{name: "to", calls: &calls})
	in := map[string]any{"v": "a"}
	prov := fake.NewScriptedTurns(
		fake.Turn{ToolUses: []llm.ToolUse{{ID: "t1", Name: "to", Input: in}}, Finish: llm.FinishToolUse},
		fake.Turn{ToolUses: []llm.ToolUse{{ID: "t2", Name: "to", Input: in}}, Finish: llm.FinishToolUse},
		fake.Turn{Text: "done", Finish: llm.FinishStop},
	)
	loop := mustNewLoop(t, Options{Provider: prov, Registry: reg, DedupEnabled: true, ToolTimeout: 5 * time.Millisecond})
	r := &recorder{}
	if _, err := loop.Run(context.Background(), userReq("q"), r.emit); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if calls != 2 {
		t.Errorf("Execute calls = %d; want 2 (a per-tool-timeout result must NOT be cached)", calls)
	}
}
