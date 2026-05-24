// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package block

import (
	"errors"
	"testing"

	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

// TestNonProductionStubs_ReturnUnsupported verifies the remaining stub
// spec-0.13 D-3 block types still return (nil, ErrUnsupportedNode).
// Promoted to production so far: Message (spec-1.7) / ToolUse (spec-1.9)
// / ToolResult (spec-1.9b) / CompactSummary (spec-1.10) / Markdown
// (spec-1.11) / Code (spec-1.12) / Diff (spec-1.13). Remaining 2 stubs
// (Banner/Progress) preserve the spec-0.13 stub contract.
//
// spec-1.12 R2 codex L1: Code removed from stub list (promoted to
// production). renderCodeBlock helper remains in code.go alongside
// the production Code{Source, Lang} block type per spec-1.12 D-4.
func TestNonProductionStubs_ReturnUnsupported(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		node RenderNode
	}{
		{"banner", Banner{}},
		{"progress", Progress{}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			buf, err := c.node.Render(Context{Cols: 80, Rows: 24})
			if buf != nil {
				t.Errorf("%s.Render: want nil Buffer, got %v", c.name, buf)
			}
			if !errors.Is(err, ErrUnsupportedNode) {
				t.Errorf("%s.Render: want ErrUnsupportedNode, got %v", c.name, err)
			}
		})
	}
}

func TestErrUnsupportedNode_Errcode(t *testing.T) {
	t.Parallel()
	if ErrUnsupportedNode.Code() != "RENDER.UNSUPPORTED_NODE" {
		t.Errorf("Code = %q", ErrUnsupportedNode.Code())
	}
	var ec errcode.Error
	if !errors.As(ErrUnsupportedNode, &ec) {
		t.Errorf("ErrUnsupportedNode should satisfy errcode.Error")
	}
}

func TestContext_ZeroValue(t *testing.T) {
	t.Parallel()
	c := Context{}
	if c.Cols != 0 || c.Rows != 0 {
		t.Errorf("zero Context: %+v", c)
	}
}
