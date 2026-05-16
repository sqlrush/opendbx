// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package streaming

import (
	"strings"
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/block"
)

type fakeStream struct{ buf strings.Builder }

func (s *fakeStream) Append(t string) { s.buf.WriteString(t) }
func (s *fakeStream) Flush() (block.RenderNode, error) {
	return block.Message{}, nil
}

func TestStream_InterfaceContract(t *testing.T) {
	t.Parallel()
	var s Stream = &fakeStream{}
	s.Append("hello")
	s.Append(" world")
	node, err := s.Flush()
	if err != nil {
		t.Errorf("Flush: %v", err)
	}
	if node == nil {
		t.Errorf("Flush returned nil node")
	}
}
