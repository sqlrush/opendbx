// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package llmapp

import (
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/block"
)

// TestMarkAssistantBullet_FirstContentOnly: the bullet lands on the first
// non-empty content Message only; thinking/empty/tool nodes are skipped and
// the bullet is consumed (pending→false) once placed.
func TestMarkAssistantBullet_FirstContentOnly(t *testing.T) {
	nodes := []block.RenderNode{
		block.Message{Empty: true},                  // thinking-only placeholder: skip
		block.Message{Text: "first answer"},         // ← gets the bullet
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
	nodes := []block.RenderNode{block.Message{Empty: true}}
	_, pending := markAssistantBullet(nodes, true)
	if !pending {
		t.Errorf("no content node → bullet must stay pending")
	}
}

// TestMarkAssistantBullet_NotPendingNoop: when not pending, nodes are
// untouched.
func TestMarkAssistantBullet_NotPendingNoop(t *testing.T) {
	nodes := []block.RenderNode{block.Message{Text: "x"}}
	out, pending := markAssistantBullet(nodes, false)
	if pending {
		t.Errorf("pending should remain false")
	}
	if out[0].(block.Message).Speaker == block.SpeakerAssistant {
		t.Errorf("not-pending must not mark any node")
	}
}
