// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package dbquery_test — spec-2.3a D-6 integration: the REAL dbquery.Tool
// wired into the REAL diagnose.Loop with a scripted fake provider and a
// fake QueryConn, plus interplay with the spec-2.3 SkillTool ToolFilter
// (a skill whose allowed-tools lists db_query).
package dbquery_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sqlrush/opendbx/internal/app/diagnose"
	"github.com/sqlrush/opendbx/internal/app/skills"
	"github.com/sqlrush/opendbx/internal/app/skills/invoke"
	"github.com/sqlrush/opendbx/internal/app/tools/dbquery"
	"github.com/sqlrush/opendbx/internal/domain/db"
	"github.com/sqlrush/opendbx/internal/domain/llm"
	"github.com/sqlrush/opendbx/internal/domain/llm/fake"
)

// fakeQueryConn implements db.QueryConn for the integration wiring.
type fakeQueryConn struct {
	result   db.QueryResult
	queryErr error
}

func (f *fakeQueryConn) Ping(context.Context) error                     { return nil }
func (f *fakeQueryConn) HealthCheck(context.Context) (db.Health, error) { return db.Health{}, nil }
func (f *fakeQueryConn) Close() error                                   { return nil }
func (f *fakeQueryConn) Query(context.Context, string, db.QueryOptions) (db.QueryResult, error) {
	return f.result, f.queryErr
}

func newDBTool(qc db.QueryConn, openErr error) *dbquery.Tool {
	return dbquery.New(func(context.Context) (db.Conn, error) {
		if openErr != nil {
			return nil, openErr
		}
		return qc, nil
	})
}

func userReq(text string) llm.Request {
	return llm.Request{
		Messages:  []llm.Message{{Role: llm.RoleUser, Content: []llm.ContentBlock{{Type: llm.BlockText, Text: text}}}},
		MaxTokens: 1024,
	}
}

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

// TestDBQuery_HappyPath — the model calls db_query, the rendered table lands
// in the transcript, and the run terminates normally.
func TestDBQuery_HappyPath(t *testing.T) {
	t.Parallel()
	qc := &fakeQueryConn{result: db.QueryResult{
		Columns: []string{"datname", "state"},
		Rows:    [][]string{{"app", "active"}, {"app", "idle"}},
	}}
	reg, err := diagnose.NewRegistry(newDBTool(qc, nil))
	if err != nil {
		t.Fatal(err)
	}
	prov := fake.NewScriptedTurns(
		fake.Turn{Finish: llm.FinishToolUse, ToolUses: []llm.ToolUse{
			{ID: "c1", Name: "db_query", Input: map[string]any{"sql": "SELECT datname, state FROM pg_stat_activity"}}}},
		fake.Turn{Text: "done", Finish: llm.FinishStop},
	)
	loop, err := diagnose.NewLoop(diagnose.Options{Provider: prov, Registry: reg, MaxTurns: 5})
	if err != nil {
		t.Fatal(err)
	}
	res, err := loop.Run(context.Background(), userReq("why so many connections"), func(context.Context, diagnose.Event) error { return nil })
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	trs := toolResults(res.Messages)
	if len(trs) != 1 || trs[0].IsError {
		t.Fatalf("tool results = %+v; want 1 success", trs)
	}
	for _, want := range []string{"datname", "active", "(2 rows)"} {
		if !strings.Contains(trs[0].Content, want) {
			t.Errorf("table missing %q:\n%s", want, trs[0].Content)
		}
	}
}

// TestDBQuery_CtxCancelIsFatal — a ctx-deadline query error must terminate
// the run on the spec-1.21 two-track path (FinishCancelled), NOT surface as
// a recoverable [DB.TIMEOUT] tool result (codex CRIT-1 end-to-end).
func TestDBQuery_CtxCancelIsFatal(t *testing.T) {
	t.Parallel()
	qc := &fakeQueryConn{queryErr: context.Canceled}
	reg, _ := diagnose.NewRegistry(newDBTool(qc, nil))
	prov := fake.NewScriptedTurns(
		fake.Turn{Finish: llm.FinishToolUse, ToolUses: []llm.ToolUse{
			{ID: "c1", Name: "db_query", Input: map[string]any{"sql": "SELECT 1"}}}},
		fake.Turn{Text: "unreached", Finish: llm.FinishStop},
	)
	loop, _ := diagnose.NewLoop(diagnose.Options{Provider: prov, Registry: reg, MaxTurns: 5})
	res, err := loop.Run(context.Background(), userReq("go"), func(context.Context, diagnose.Event) error { return nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run err = %v; want context.Canceled (fatal two-track)", err)
	}
	if res.FinishReason != llm.FinishCancelled {
		t.Errorf("FinishReason = %v; want FinishCancelled", res.FinishReason)
	}
	// No recoverable tool result was committed for the cancelled call.
	for _, tr := range toolResults(res.Messages) {
		if strings.Contains(tr.Content, "DB.TIMEOUT") {
			t.Error("ctx cancel leaked as a recoverable DB.TIMEOUT tool result")
		}
	}
}

// TestDBQuery_RecoverableError — a non-ctx query error (read-only violation)
// is a recoverable tool result and the run continues.
func TestDBQuery_RecoverableError(t *testing.T) {
	t.Parallel()
	qc := &fakeQueryConn{queryErr: db.ErrReadOnlyViolation}
	reg, _ := diagnose.NewRegistry(newDBTool(qc, nil))
	prov := fake.NewScriptedTurns(
		fake.Turn{Finish: llm.FinishToolUse, ToolUses: []llm.ToolUse{
			{ID: "c1", Name: "db_query", Input: map[string]any{"sql": "DELETE FROM t"}}}},
		fake.Turn{Text: "ok, read-only understood", Finish: llm.FinishStop},
	)
	loop, _ := diagnose.NewLoop(diagnose.Options{Provider: prov, Registry: reg, MaxTurns: 5})
	res, err := loop.Run(context.Background(), userReq("delete stuff"), func(context.Context, diagnose.Event) error { return nil })
	if err != nil {
		t.Fatalf("recoverable error must not terminate: %v", err)
	}
	trs := toolResults(res.Messages)
	if len(trs) != 1 || !trs[0].IsError || !strings.Contains(trs[0].Content, "DB.READONLY_VIOLATION") {
		t.Fatalf("want 1 recoverable readonly result: %+v", trs)
	}
}

// TestDBQuery_DedupCachedReplay — the same SQL within the dedup window
// returns a byte-identical cached result.
func TestDBQuery_DedupCachedReplay(t *testing.T) {
	t.Parallel()
	qc := &fakeQueryConn{result: db.QueryResult{Columns: []string{"c"}, Rows: [][]string{{"1"}}}}
	reg, _ := diagnose.NewRegistry(newDBTool(qc, nil))
	prov := fake.NewScriptedTurns(
		fake.Turn{Finish: llm.FinishToolUse, ToolUses: []llm.ToolUse{
			{ID: "c1", Name: "db_query", Input: map[string]any{"sql": "SELECT 1"}}}},
		fake.Turn{Finish: llm.FinishToolUse, ToolUses: []llm.ToolUse{
			{ID: "c2", Name: "db_query", Input: map[string]any{"sql": "SELECT 1"}}}},
		fake.Turn{Text: "done", Finish: llm.FinishStop},
	)
	loop, _ := diagnose.NewLoop(diagnose.Options{Provider: prov, Registry: reg, MaxTurns: 5, DedupEnabled: true})
	var c2Cached bool
	emit := func(_ context.Context, e diagnose.Event) error {
		if e.Kind == diagnose.EventToolResult && e.ToolResult.ToolUseID == "c2" {
			c2Cached = e.Cached
		}
		return nil
	}
	res, err := loop.Run(context.Background(), userReq("go"), emit)
	if err != nil {
		t.Fatal(err)
	}
	if !c2Cached {
		t.Error("second identical SELECT should be a dedup cache hit")
	}
	trs := toolResults(res.Messages)
	if len(trs) != 2 || trs[0].Content != trs[1].Content {
		t.Errorf("cached replay not byte-identical:\n%q\nvs\n%q", trs[0].Content, trs[1].Content)
	}
}

// TestDBQuery_SkillScopeAllows — a skill whose allowed-tools lists db_query
// keeps it callable inside the skill scope (spec-2.3 ToolFilter interplay).
func TestDBQuery_SkillScopeAllows(t *testing.T) {
	t.Parallel()
	qc := &fakeQueryConn{result: db.QueryResult{Columns: []string{"c"}, Rows: [][]string{{"x"}}}}
	skill := skills.Skill{
		Schema: skills.Schema{Name: "db-doctor", Description: "DB diagnostics.", AllowedTools: "db_query"},
		Body:   "Use db_query to inspect pg_stat_activity.",
	}
	st, err := invoke.NewSkillTool([]skills.Skill{skill})
	if err != nil {
		t.Fatal(err)
	}
	reg, err := diagnose.NewRegistry(st, newDBTool(qc, nil))
	if err != nil {
		t.Fatal(err)
	}
	prov := fake.NewScriptedTurns(
		// Enter the skill scope (allowed-tools: db_query).
		fake.Turn{Finish: llm.FinishToolUse, ToolUses: []llm.ToolUse{
			{ID: "c1", Name: "Skill", Input: map[string]any{"skill": "db-doctor"}}}},
		// db_query is in scope → executes.
		fake.Turn{Finish: llm.FinishToolUse, ToolUses: []llm.ToolUse{
			{ID: "c2", Name: "db_query", Input: map[string]any{"sql": "SELECT 1"}}}},
		fake.Turn{Text: "done", Finish: llm.FinishStop},
	)
	loop, _ := diagnose.NewLoop(diagnose.Options{Provider: prov, Registry: reg, MaxTurns: 5})
	res, err := loop.Run(context.Background(), userReq("diagnose"), func(context.Context, diagnose.Event) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	trs := toolResults(res.Messages)
	// c1 = skill body, c2 = db_query table (not SCOPE_TOOL_DENIED).
	if len(trs) != 2 || trs[1].IsError || !strings.Contains(trs[1].Content, "(1 rows)") {
		t.Errorf("db_query should run inside the db-doctor scope: %+v", trs)
	}
}

// TestDBQuery_SkillScopeDenied — a skill whose allowed-tools does NOT list
// db_query keeps it OUT of scope: an attempt returns SCOPE_TOOL_DENIED, not
// an execution (spec-2.3a D-6 / spec-2.3 ToolFilter regression).
func TestDBQuery_SkillScopeDenied(t *testing.T) {
	t.Parallel()
	qc := &fakeQueryConn{result: db.QueryResult{Columns: []string{"c"}, Rows: [][]string{{"x"}}}}
	skill := skills.Skill{
		Schema: skills.Schema{Name: "clock-only", Description: "Time only.", AllowedTools: "clock"},
		Body:   "Use clock.",
	}
	st, err := invoke.NewSkillTool([]skills.Skill{skill})
	if err != nil {
		t.Fatal(err)
	}
	reg, err := diagnose.NewRegistry(st, newDBTool(qc, nil), diagnose.ClockTool{})
	if err != nil {
		t.Fatal(err)
	}
	prov := fake.NewScriptedTurns(
		fake.Turn{Finish: llm.FinishToolUse, ToolUses: []llm.ToolUse{
			{ID: "c1", Name: "Skill", Input: map[string]any{"skill": "clock-only"}}}},
		fake.Turn{Finish: llm.FinishToolUse, ToolUses: []llm.ToolUse{
			{ID: "c2", Name: "db_query", Input: map[string]any{"sql": "SELECT 1"}}}},
		fake.Turn{Text: "denied, ok", Finish: llm.FinishStop},
	)
	loop, _ := diagnose.NewLoop(diagnose.Options{Provider: prov, Registry: reg, MaxTurns: 5})
	res, err := loop.Run(context.Background(), userReq("go"), func(context.Context, diagnose.Event) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	trs := toolResults(res.Messages)
	if len(trs) != 2 || !trs[1].IsError || !strings.Contains(trs[1].Content, "SKILL.SCOPE_TOOL_DENIED") {
		t.Errorf("db_query outside scope must be denied: %+v", trs)
	}
}
