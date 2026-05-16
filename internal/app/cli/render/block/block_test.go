// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package block

import (
	"errors"
	"testing"

	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

// TestAll8Stubs_ReturnUnsupported verifies each of the 8 spec-0.13 D-3
// block type stubs returns (nil, ErrUnsupportedNode) from Render().
func TestAll8Stubs_ReturnUnsupported(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		node RenderNode
	}{
		{"message", Message{}},
		{"toolcall", Toolcall{}},
		{"compact", Compact{}},
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
