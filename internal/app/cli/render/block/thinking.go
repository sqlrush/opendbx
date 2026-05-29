// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File thinking.go — render-only Thinking block (spec-1.20.2 D-5).
// Thinking is transient UI state for provider reasoning/thinking deltas. It is
// intentionally NOT an llm.BlockType and must never enter provider/session wire
// history.

package block

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
)

const thinkingOnlyStripMarker = "[模型仅返回 thinking;如需查看请设置 strip_think: false]"

// Thinking renders provider-side reasoning/thinking content as an independent
// UI-only block. Collapsed=true shows a compact one-line summary; false renders
// the raw content dimmed and wrapped. It satisfies RenderNode only.
type Thinking struct {
	Content   string
	Collapsed bool
}

var _ RenderNode = Thinking{}

// ThinkingOnlyStripMarker returns the explicit marker used when a model emits
// only thinking content while strip_think is enabled.
func ThinkingOnlyStripMarker() string { return thinkingOnlyStripMarker }

// Render implements RenderNode.
func (t Thinking) Render(ctx Context) (buffer.Buffer, error) {
	if ctx.Cols <= 0 {
		return measureOnlyBuf(0, 0), nil
	}
	content := strings.TrimSpace(t.Content)
	if content == "" {
		return measureOnlyBuf(ctx.Cols, 0), nil
	}
	text := content
	if t.Collapsed {
		text = thinkingSummary(content)
	}
	lines := wrap(text, ctx.Cols, ctx.Wrap)
	if len(lines) == 0 {
		return measureOnlyBuf(ctx.Cols, 0), nil
	}
	if ctx.MeasureOnly {
		return measureOnlyBuf(ctx.Cols, len(lines)), nil
	}
	buf, err := buffer.NewGrid(ctx.Cols, len(lines))
	if err != nil {
		return measureOnlyBuf(ctx.Cols, len(lines)), nil
	}
	st := themeOrDefault(ctx.Theme).Style(StyleDimmed)
	for y, line := range lines {
		writeTextRow(buf, 0, y, line, st, ctx.Cols)
	}
	return buf, nil
}

func thinkingSummary(content string) string {
	fields := strings.Fields(content)
	count := len(fields)
	unit := "tokens"
	if count == 0 {
		count = utf8.RuneCountInString(content)
		unit = "chars"
	}
	return fmt.Sprintf("[思考中] %d %s", count, unit)
}
