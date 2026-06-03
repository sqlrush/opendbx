// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File snapshot_test.go — spec-1.23 D-3: RunSnapshot capture from the Loop
// Event stream, sealed at EventFinish.

package llmapp

import (
	"context"
	"testing"
	"time"

	"github.com/sqlrush/opendbx/internal/app/diagnose"
	"github.com/sqlrush/opendbx/internal/app/report"
	"github.com/sqlrush/opendbx/internal/domain/llm"
	"github.com/sqlrush/opendbx/internal/domain/llm/fake"
)

// TestModel_CapturesRunSnapshot — a completed tool round-trip diagnosis seals a
// snapshot with the prompt, final answer, and tool timeline.
func TestModel_CapturesRunSnapshot(t *testing.T) {
	t.Parallel()
	reg, err := diagnose.NewRegistry(diagnose.EchoTool{})
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	prov := fake.NewScriptedTurns(
		fake.Turn{ToolUses: []llm.ToolUse{{ID: "c1", Name: "echo", Input: map[string]any{"msg": "ping"}}}, Finish: llm.FinishToolUse},
		fake.Turn{Text: "diagnosis done: ok", Finish: llm.FinishStop},
	)
	m := New(prov, Options{ModelName: "loop", MaxTokens: 1024, Registry: reg})
	m = typeAndModel(t, m, "whyslow")
	final := runStream(t, m)

	snap := final.lastSnapshot
	if snap == nil {
		t.Fatal("lastSnapshot nil after a completed run")
	}
	if snap.Prompt != "whyslow" {
		t.Errorf("Prompt = %q; want whyslow", snap.Prompt)
	}
	if snap.FinalAnswer != "diagnosis done: ok" {
		t.Errorf("FinalAnswer = %q", snap.FinalAnswer)
	}
	if len(snap.ToolTimeline) != 1 || snap.ToolTimeline[0].Name != "echo" {
		t.Fatalf("ToolTimeline = %+v; want 1 echo", snap.ToolTimeline)
	}
	if snap.ToolTimeline[0].Result == "" {
		t.Error("tool result not captured into timeline")
	}
	if snap.FinishStatus != llm.FinishStop {
		t.Errorf("FinishStatus = %v; want FinishStop", snap.FinishStatus)
	}
	if snap.StartedAt.IsZero() || snap.FinishedAt.IsZero() {
		t.Error("run timing not captured")
	}
}

// TestModel_CancelledRun_KeepsPriorSnapshot — a cancelled run does NOT overwrite
// a prior good snapshot ("last completed", not "last attempted" — architect L-2).
func TestModel_CancelledRun_KeepsPriorSnapshot(t *testing.T) {
	t.Parallel()
	good := &report.RunSnapshot{Prompt: "good"}
	m := &Model{lastSnapshot: good}

	out, _ := m.handleControl(streamControlMsg{Finish: llm.FinishCancelled, Snapshot: &report.RunSnapshot{Prompt: "cancelled"}})
	if got := out.(*Model).lastSnapshot; got != good {
		t.Errorf("cancelled run overwrote snapshot: %+v", got)
	}

	// spec-1.21.1 R-fix H-2: a FinishError wrapping context.Canceled is ALSO a
	// cancel (it renders [已取消]) and must NOT overwrite the snapshot either —
	// the marker render + snapshot guard share isCancelledFinish.
	outC, _ := m.handleControl(streamControlMsg{
		Finish:   llm.FinishError,
		Err:      context.Canceled,
		Snapshot: &report.RunSnapshot{Prompt: "err-cancelled"},
	})
	mc := outC.(*Model)
	if got := mc.lastSnapshot; got != good {
		t.Errorf("FinishError+context.Canceled overwrote snapshot: %+v", got)
	}
	if !hasNode(mc, "[已取消]") {
		t.Errorf("FinishError+context.Canceled should render [已取消]: %v", nodeTexts(mc))
	}

	out2, _ := m.handleControl(streamControlMsg{Finish: llm.FinishStop, Snapshot: &report.RunSnapshot{Prompt: "done"}})
	if got := out2.(*Model).lastSnapshot; got == nil || got.Prompt != "done" {
		t.Errorf("completed run did not adopt snapshot: %+v", got)
	}
}

// TestSnapshotBuilder_MultiToolPairing — multiple tools per turn (1:N): results
// join their calls by ToolUseID even when delivered out of call order.
func TestSnapshotBuilder_MultiToolPairing(t *testing.T) {
	t.Parallel()
	b := newSnapshotBuilder("q", func() time.Time { return time.Unix(100, 0) })
	b.addToolCall(&llm.ToolUse{ID: "a", Name: "t1", Input: map[string]any{"x": 1}})
	b.addToolCall(&llm.ToolUse{ID: "b", Name: "t2"})
	// results arrive in a different order than the calls.
	b.addToolResult(&llm.ToolResult{ToolUseID: "b", Content: "res-b"}, true)
	b.addToolResult(&llm.ToolResult{ToolUseID: "a", Content: "res-a", IsError: true}, false)
	b.addText("ans")
	snap := b.seal(diagnose.Event{Turn: 3, Finish: llm.FinishStop})

	if len(snap.ToolTimeline) != 2 {
		t.Fatalf("timeline len = %d; want 2", len(snap.ToolTimeline))
	}
	te0 := snap.ToolTimeline[0]
	if te0.Name != "t1" || te0.Result != "res-a" || !te0.IsError || te0.Cached {
		t.Errorf("t1 mispaired: %+v", te0)
	}
	te1 := snap.ToolTimeline[1]
	if te1.Name != "t2" || te1.Result != "res-b" || te1.IsError || !te1.Cached {
		t.Errorf("t2 mispaired: %+v", te1)
	}
	if snap.FinalAnswer != "ans" || snap.Turns != 3 {
		t.Errorf("seal wrong: %+v", snap)
	}
}

// TestSnapshotBuilder_OrphanResult_Ignored — an unmatched tool_result is
// dropped, not a panic.
func TestSnapshotBuilder_OrphanResult_Ignored(t *testing.T) {
	t.Parallel()
	b := newSnapshotBuilder("q", func() time.Time { return time.Unix(0, 0) })
	b.addToolResult(&llm.ToolResult{ToolUseID: "ghost", Content: "x"}, false) // no matching call
	snap := b.seal(diagnose.Event{Finish: llm.FinishStop})
	if len(snap.ToolTimeline) != 0 {
		t.Errorf("orphan result created a timeline entry: %+v", snap.ToolTimeline)
	}
}
