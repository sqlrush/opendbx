// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package scrollback

import (
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/block"
)

type fakeScrollback struct {
	items []block.RenderNode
}

func (f *fakeScrollback) Push(n block.RenderNode) { f.items = append(f.items, n) }
func (f *fakeScrollback) Range(start, end int) []block.RenderNode {
	if start < 0 {
		start = 0
	}
	if end > len(f.items) {
		end = len(f.items)
	}
	return f.items[start:end]
}
func (f *fakeScrollback) Len() int { return len(f.items) }

func TestScrollback_InterfaceContract(t *testing.T) {
	t.Parallel()
	var sb Scrollback = &fakeScrollback{}
	sb.Push(block.Message{})
	sb.Push(block.Code{})
	if sb.Len() != 2 {
		t.Errorf("Len = %d want 2", sb.Len())
	}
	r := sb.Range(0, 2)
	if len(r) != 2 {
		t.Errorf("Range = %d items want 2", len(r))
	}
}
