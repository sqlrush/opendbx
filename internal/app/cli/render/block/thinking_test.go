// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package block

import (
	"strings"
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
)

func TestThinking_RenderCollapsed(t *testing.T) {
	t.Parallel()
	buf, err := Thinking{Content: "step one step two", Collapsed: true}.Render(Context{Cols: 40, Rows: 1})
	if err != nil {
		t.Fatalf("Render err: %v", err)
	}
	got := gridTextForThinkingTest(buf)
	if !strings.Contains(got, "[思考中] 4 tokens") {
		t.Fatalf("collapsed thinking = %q", got)
	}
}

func TestThinking_RenderExpanded(t *testing.T) {
	t.Parallel()
	buf, err := Thinking{Content: "推理内容", Collapsed: false}.Render(Context{Cols: 40, Rows: 1})
	if err != nil {
		t.Fatalf("Render err: %v", err)
	}
	got := gridTextForThinkingTest(buf)
	if !strings.Contains(got, "推理内容") {
		t.Fatalf("expanded thinking = %q", got)
	}
}

func TestThinking_RenderEmpty(t *testing.T) {
	t.Parallel()
	buf, err := Thinking{Content: "   ", Collapsed: true}.Render(Context{Cols: 40, Rows: 1})
	if err != nil {
		t.Fatalf("Render err: %v", err)
	}
	_, rows := buf.Size()
	if rows != 0 {
		t.Fatalf("empty thinking rows = %d, want 0", rows)
	}
}

func gridTextForThinkingTest(b buffer.Buffer) string {
	cols, rows := b.Size()
	var sb strings.Builder
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			c := b.Cell(x, y)
			if c.Ch != 0 && !buffer.IsContinuation(c) {
				sb.WriteRune(c.Ch)
			}
		}
	}
	return sb.String()
}
