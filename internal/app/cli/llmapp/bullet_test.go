// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package llmapp

import (
	"context"
	"strings"
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/block"
	"github.com/sqlrush/opendbx/internal/app/cli/render/streaming"
	"github.com/sqlrush/opendbx/internal/domain/llm"
	"github.com/sqlrush/opendbx/internal/domain/llm/fake"
)

// TestHandleControl_DrainsTextBeforeToolUse is the regression for codex
// D5-HIGH-1: pending assistant text must be flushed into scrollback (carrying
// the ⏺ bullet) BEFORE a tool-use node is appended, so the prose that
// preceded the tool call orders first — not after the tool tree on a later
// View frame.
func TestHandleControl_DrainsTextBeforeToolUse(t *testing.T) {
	t.Parallel()
	ts := streaming.NewTokenStream(context.Background())
	ts.Append("checking the slow query\n") // newline-completed → drainable

	m := New(fake.New(), Options{ModelName: "fake"})
	m.stream = ts
	m.control = make(chan streamControlMsg, 4)
	m.streaming = true
	m.assistantBulletPending = true

	mm, _ := m.handleControl(streamControlMsg{
		ToolUse: &llm.ToolUse{ID: "c1", Name: "echo", Input: map[string]any{"q": "x"}},
	})
	sb := mm.(*Model).scrollback
	if len(sb) < 2 {
		t.Fatalf("expected [assistant text, tooluse], got %d nodes", len(sb))
	}
	msg, ok := sb[0].(block.Message)
	if !ok {
		t.Fatalf("node 0 = %T; want assistant text Message before the tool node", sb[0])
	}
	if msg.Speaker != block.SpeakerAssistant {
		t.Errorf("flushed assistant text should carry SpeakerAssistant bullet, got %v", msg.Speaker)
	}
	if !strings.Contains(msg.Text, "checking the slow query") {
		t.Errorf("node 0 text = %q; want the pending prose", msg.Text)
	}
	if _, ok := sb[1].(block.ToolUse); !ok {
		t.Errorf("node 1 = %T; want block.ToolUse AFTER the text", sb[1])
	}
}

// TestMarkAssistantBullet_FirstContentOnly: the bullet lands on the first
// non-empty content Message only; thinking/empty/tool nodes are skipped and
// the bullet is consumed (pending→false) once placed.
func TestMarkAssistantBullet_FirstContentOnly(t *testing.T) {
	t.Parallel()
	nodes := []block.RenderNode{
		block.Message{Empty: true},                   // thinking-only placeholder: skip
		block.Message{Text: "first answer"},          // ← gets the bullet
		block.Message{Text: "second line same turn"}, // no bullet (per-turn)
	}
	out, pending := markAssistantBullet(nodes, true)
	if pending {
		t.Errorf("bullet should be consumed (pending=false) after placement")
	}
	m0 := out[0].(block.Message)
	m1 := out[1].(block.Message)
	m2 := out[2].(block.Message)
	if m0.Speaker == block.SpeakerAssistant {
		t.Errorf("empty placeholder must NOT get the bullet")
	}
	if m1.Speaker != block.SpeakerAssistant {
		t.Errorf("first content node should get SpeakerAssistant, got %v", m1.Speaker)
	}
	if m2.Speaker == block.SpeakerAssistant {
		t.Errorf("second content node must NOT get a bullet (per-turn)")
	}
}

// TestMarkAssistantBullet_StaysPendingNoContent: a drain with no content
// Message (e.g. thinking-only) keeps the bullet pending for a later drain in
// the same turn.
func TestMarkAssistantBullet_StaysPendingNoContent(t *testing.T) {
	t.Parallel()
	nodes := []block.RenderNode{block.Message{Empty: true}}
	_, pending := markAssistantBullet(nodes, true)
	if !pending {
		t.Errorf("no content node → bullet must stay pending")
	}
}

// TestMarkAssistantBullet_NotPendingNoop: when not pending, nodes are
// untouched.
func TestMarkAssistantBullet_NotPendingNoop(t *testing.T) {
	t.Parallel()
	nodes := []block.RenderNode{block.Message{Text: "x"}}
	out, pending := markAssistantBullet(nodes, false)
	if pending {
		t.Errorf("pending should remain false")
	}
	if out[0].(block.Message).Speaker == block.SpeakerAssistant {
		t.Errorf("not-pending must not mark any node")
	}
}
