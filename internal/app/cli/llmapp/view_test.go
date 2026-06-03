// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package llmapp

import (
	"strings"
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/program"
	"github.com/sqlrush/opendbx/internal/app/cli/render/terminal"
	"github.com/sqlrush/opendbx/internal/domain/llm"
	"github.com/sqlrush/opendbx/internal/domain/llm/fake"
)

func TestModel_Init(t *testing.T) {
	t.Parallel()
	if New(fake.New(), Options{}).Init() != nil {
		t.Error("Init should return nil Cmd")
	}
}

func TestModel_View_RendersScrollback(t *testing.T) {
	t.Parallel()
	m := typeAndModel(t, newFakeModel(fake.Scripted("answer", llm.FinishStop)), "ask")
	final := runStream(t, m)
	buf := final.View(80, 10)
	if buf == nil {
		t.Fatalf("View returned nil")
	}
	txt := gridText(buf)
	if txt == "" {
		t.Errorf("View produced empty grid; want rendered scrollback")
	}
}

func TestModel_View_PreservesWideRunes(t *testing.T) {
	t.Parallel()
	m := typeAndModel(t, newFakeModel(fake.Scripted("中文 PostgreSQL MVCC", llm.FinishStop)), "ask")
	final := runStream(t, m)
	txt := gridText(final.View(120, 10))
	if !strings.Contains(txt, "中文 PostgreSQL MVCC") {
		t.Fatalf("View text = %q; want CJK + ASCII preserved", txt)
	}
}

func TestModel_View_InFlightPreviewNoPanic(t *testing.T) {
	t.Parallel()
	// Submit with a no-finish chunk so a stream is in flight; View should
	// render without mutating or draining preview state.
	m := typeAndModel(t, newFakeModel(fake.New(llm.Chunk{Token: "x"})), "q")
	mm, cmd := m.Update(keyAction(terminal.KeyEnter, 0))
	_ = cmd
	streamingModel := mm.(*Model)
	_ = streamingModel.View(80, 10)
}

func TestModel_View_SmallGrid(t *testing.T) {
	t.Parallel()
	m := New(fake.New(), Options{})
	if m.View(0, 0) != nil {
		t.Error("View(0,0) should return nil (invalid grid)")
	}
}

func TestModel_CursorMovement(t *testing.T) {
	t.Parallel()
	m := typeAndModel(t, newFakeModel(fake.New()), "abc")
	cur := program.Model(m)
	cur, _ = cur.Update(keyAction(terminal.KeyLeft, 0))
	cur, _ = cur.Update(keyAction(terminal.KeyCtrlA, 0)) // Home
	if cur.(*Model).InputState().Cursor != 0 {
		t.Errorf("cursor after Home = %d; want 0", cur.(*Model).InputState().Cursor)
	}
	cur, _ = cur.Update(keyAction(terminal.KeyCtrlE, 0)) // End
	if cur.(*Model).InputState().Cursor != 3 {
		t.Errorf("cursor after End = %d; want 3", cur.(*Model).InputState().Cursor)
	}
}

func TestModel_Cancel_NoStream(t *testing.T) {
	t.Parallel()
	m := typeAndModel(t, newFakeModel(fake.New()), "draft")
	next, cmd := m.Update(program.CancelCmdMsg{})
	if cmd != nil {
		t.Errorf("cancel with no stream should return nil cmd")
	}
	if next.(*Model).InputState().Buffer != "" {
		t.Errorf("cancel should clear buffer; got %q", next.(*Model).InputState().Buffer)
	}
}

func TestModel_Cancel_InFlight(t *testing.T) {
	t.Parallel()
	// Start a stream (no-finish chunk keeps it in flight), then cancel.
	m := typeAndModel(t, newFakeModel(fake.New(llm.Chunk{Token: "x"})), "q")
	mm, _ := m.Update(keyAction(terminal.KeyEnter, 0))
	streamingModel := mm.(*Model)
	_, cmd := streamingModel.Update(program.CancelCmdMsg{})
	if cmd == nil {
		t.Fatalf("cancel in-flight should return a cancel Cmd")
	}
	if msg := cmd(); msg != nil {
		t.Errorf("cancel Cmd should return nil Msg; got %v", msg)
	}
}

func TestModel_Cleanup(t *testing.T) {
	t.Parallel()
	// Idle: nil cleanup.
	if New(fake.New(), Options{}).Cleanup() != nil {
		t.Error("idle Cleanup should be nil")
	}
	// In-flight: cleanup cancels.
	m := typeAndModel(t, newFakeModel(fake.New(llm.Chunk{Token: "x"})), "q")
	mm, _ := m.Update(keyAction(terminal.KeyEnter, 0))
	c := mm.(*Model).Cleanup()
	if c == nil {
		t.Fatalf("in-flight Cleanup should return a cancel Cmd")
	}
	if msg := c(); msg != nil {
		t.Errorf("Cleanup Cmd should return nil Msg")
	}
}

func TestModel_StatusSegments_StreamingIndicator(t *testing.T) {
	t.Parallel()
	m := typeAndModel(t, newFakeModel(fake.New(llm.Chunk{Token: "x"})), "q")
	mm, _ := m.Update(keyAction(terminal.KeyEnter, 0))
	segs := mm.(*Model).StatusSegments()
	if len(segs) != 2 || segs[1].Text != "●" {
		t.Errorf("streaming status = %+v; want [model ●]", segs)
	}
}

func TestModel_StatusSegments_ProviderNameFallback(t *testing.T) {
	t.Parallel()
	m := New(fake.New(), Options{}) // no ModelName
	if m.StatusSegments()[0].Text != "fake" {
		t.Errorf("status[0] = %q; want provider name fallback 'fake'", m.StatusSegments()[0].Text)
	}
}

// TestToolUsePlaceholder_Empty was retired in spec-1.21 T-8: the
// toolUsePlaceholder helper no longer exists — Loop now executes the
// tool and emits block.ToolUse / block.ToolResult render nodes directly
// (spec-1.21 D-6). Coverage of the multi-turn tool-render path lives in
// model_test.go TestModel_LoopAppendsToolBlocks.
