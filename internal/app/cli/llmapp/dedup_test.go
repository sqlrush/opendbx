// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File dedup_test.go — spec-1.22 T-5b: the dedup Cached flag survives the
// full 4-hop UI plumbing Event.Cached → streamControlMsg.Cached →
// handleControl → block.ToolResult.Cached, end-to-end through llmapp.

package llmapp

import (
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/block"
	"github.com/sqlrush/opendbx/internal/app/diagnose"
	"github.com/sqlrush/opendbx/internal/domain/llm"
	"github.com/sqlrush/opendbx/internal/domain/llm/fake"
)

// TestModel_DedupHit_MarksSecondToolResultCached drives two identical echo
// calls with dedup enabled and asserts the second rendered block.ToolResult
// carries Cached=true while the first does not — proving the flag travels the
// whole UI chain. It also checks the cached content is byte-identical to the
// fresh result (no wire marker — invariant #2 / § 3.6 errata).
func TestModel_DedupHit_MarksSecondToolResultCached(t *testing.T) {
	t.Parallel()
	reg, err := diagnose.NewRegistry(diagnose.EchoTool{})
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	in := map[string]any{"msg": "ping"}
	prov := fake.NewScriptedTurns(
		fake.Turn{ToolUses: []llm.ToolUse{{ID: "c1", Name: "echo", Input: in}}, Finish: llm.FinishToolUse},
		fake.Turn{ToolUses: []llm.ToolUse{{ID: "c2", Name: "echo", Input: in}}, Finish: llm.FinishToolUse},
		fake.Turn{Text: "all done", Finish: llm.FinishStop},
	)
	m := New(prov, Options{ModelName: "loop", MaxTokens: 1024, Registry: reg, DedupEnabled: true})
	m = typeAndModel(t, m, "go")
	final := runStream(t, m)

	var trs []block.ToolResult
	for _, n := range final.scrollback {
		if tr, ok := n.(block.ToolResult); ok {
			trs = append(trs, tr)
		}
	}
	if len(trs) != 2 {
		t.Fatalf("block.ToolResult count = %d; want 2 (%v)", len(trs), nodeTexts(final))
	}
	if trs[0].Cached {
		t.Error("first tool result must NOT be Cached")
	}
	if !trs[1].Cached {
		t.Error("second identical tool result MUST be Cached (4-hop Event→streamControlMsg→handleControl→block)")
	}
	if trs[0].Content != trs[1].Content {
		t.Errorf("cached content drifted from fresh run (must be byte-identical, no marker): %v vs %v", trs[0].Content, trs[1].Content)
	}
}

// TestModel_DedupDisabled_NeverCached — without DedupEnabled the second call
// renders a fresh (non-cached) result, matching spec-1.21 behavior.
func TestModel_DedupDisabled_NeverCached(t *testing.T) {
	t.Parallel()
	reg, err := diagnose.NewRegistry(diagnose.EchoTool{})
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	in := map[string]any{"msg": "ping"}
	prov := fake.NewScriptedTurns(
		fake.Turn{ToolUses: []llm.ToolUse{{ID: "c1", Name: "echo", Input: in}}, Finish: llm.FinishToolUse},
		fake.Turn{ToolUses: []llm.ToolUse{{ID: "c2", Name: "echo", Input: in}}, Finish: llm.FinishToolUse},
		fake.Turn{Text: "all done", Finish: llm.FinishStop},
	)
	m := New(prov, Options{ModelName: "loop", MaxTokens: 1024, Registry: reg}) // DedupEnabled omitted = false
	m = typeAndModel(t, m, "go")
	final := runStream(t, m)

	for _, n := range final.scrollback {
		if tr, ok := n.(block.ToolResult); ok && tr.Cached {
			t.Error("dedup disabled: no tool result may be Cached")
		}
	}
}
