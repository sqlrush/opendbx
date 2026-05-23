// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package block

import (
	"errors"
	"testing"

	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

// TestNonProductionStubs_ReturnUnsupported verifies the 6 stub
// spec-0.13 D-3 block type stubs still return (nil, ErrUnsupportedNode).
// spec-1.7 promoted Message to production; spec-1.9 promotes ToolUse
// (was the old Toolcall stub) to production. The remaining 6 stubs
// (Compact/Markdown/Code/Diff/Banner/Progress) preserve the spec-0.13
// stub contract; future spec-1.x produces each.
//
// **Code is intentionally included here** — code.go hosts the
// renderCodeBlock helper used by Message but Code.Render itself remains
// unsupported until spec-1.12 code-highlight-block.
func TestNonProductionStubs_ReturnUnsupported(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		node RenderNode
	}{
		{"markdown", Markdown{}},
		{"code", Code{}},
		{"diff", Diff{}},
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
