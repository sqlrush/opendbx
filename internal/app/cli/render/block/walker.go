// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File walker.go — spec-1.11 D-3 AST walker per-node renderer.
//
// Per HIGH-2 design: walker accepts goldmark *ast.Node + source bytes +
// ctx + theme, materializes []rowSpec via per-node handlers, then
// builds final buffer.Buffer via buildBuffer (no intermediate styledRow
// exported type; rowSpec/styledSpan/WrapHint are walker-local per
// § 3.2.1 R3.1 lock).
//
// Visual baseline (R3 codex verified B-29 markdown.ts + figures.ts):
//   - heading: no prefix glyph; bold/permission-colored
//   - unordered list: "- " (CC standard, not "• " U+2022)
//   - blockquote: "▎ " U+258E left rail (per CC figures)
//   - HR: "---" dimmed (not "─" U+2500)
//   - strikethrough: literal ~~text~~ (CC disables tokenizer)
//   - raw HTML / del nodes: dropped (CC formatToken returns empty)
//   - links: visible text + dim URL fallback (OSC8 deferred ❌-8)
//   - fence: delegate to spec-1.7 renderCodeBlock(ctx, lang, body)

package block

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
	"github.com/sqlrush/opendbx/internal/app/cli/render/style"
	"github.com/sqlrush/opendbx/internal/app/cli/render/width"

	"github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
)

// styledSpan represents a sub-row range with a specific style override.
// walker-local helper for inline emphasis / code-span / link styling
// (R3.1 § 3.2.1 lock).
type styledSpan struct {
	start, end int // half-open [start, end) byte offsets into rowSpec.text
	style      StyleKind
}

// WrapHint signals walker-side intent to the wrap pass. 3-state enum
// sufficient for spec-1.11 scope (R3.1 § 3.2.1 lock).
type WrapHint int

const (
	// WrapHintBreak — hard break (end of block element).
	WrapHintBreak WrapHint = iota
	// WrapHintContinue — soft continue (inside paragraph).
	WrapHintContinue
	// WrapHintKeep — keep-together hint (table row / fence line).
	WrapHintKeep
)

// rowSpec accumulates a single rendered line. NOT exported; walker
// materializes []rowSpec → buffer.Buffer at the end of walk() per
// HIGH-2 design (R3.1 § 3.2.1).
type rowSpec struct {
	text     string
	style    StyleKind
	spans    []styledSpan
	cells    []buffer.Cell // optional raw styled cells, used for spec-1.7 code delegate rows
	wrapHint WrapHint
}

// walkMarkdown is the entry point for the AST walker (called from
// Markdown.Render). Returns buffer.Buffer directly.
func walkMarkdown(root ast.Node, source []byte, ctx Context, theme StyleTheme) buffer.Buffer {
	w := &mdWalker{src: source, ctx: ctx, theme: theme}
	w.walkBlock(root)
	return w.buildBuffer()
}

// mdWalker holds per-walk state. NOT exported.
type mdWalker struct {
	src   []byte
	ctx   Context
	theme StyleTheme
	rows  []rowSpec
}

// walkBlock dispatches a block-level node to its renderer.
func (w *mdWalker) walkBlock(n ast.Node) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch t := c.(type) {
		case *ast.Heading:
			w.renderHeading(t)
		case *ast.Paragraph:
			w.renderParagraph(t)
		case *ast.List:
			w.renderList(t, 0)
		case *ast.Blockquote:
			w.renderBlockquote(t)
		case *ast.FencedCodeBlock:
			w.renderFenceBlock(t)
		case *ast.CodeBlock:
			// Indented code block — treat like a fence with no lang.
			w.renderIndentedCodeBlock(t)
		case *ast.ThematicBreak:
			w.renderHR()
		case *extast.Table:
			w.renderTable(t)
		case *ast.HTMLBlock:
			// ❌-4 + I-5: raw HTML block dropped per CC formatToken.
			continue
		default:
			// Unknown block type — skip silently. Goldmark may surface
			// extension nodes (footnote / definition list); per ❌-7
			// these aren't supported in spec-1.11.
			continue
		}
	}
}

// renderHeading emits a single heading row. CC formatToken heading
// (B-29 markdown.ts) emits bold/permission-colored text with NO prefix
// glyph; depth controls style strength only.
func (w *mdWalker) renderHeading(h *ast.Heading) {
	text := w.extractInlineText(h)
	if text == "" {
		return
	}
	// All depths use StyleHeading (bold + permission color); future
	// fixture verify may differentiate per-depth.
	w.rows = append(w.rows, rowSpec{
		text:     text,
		style:    StyleHeading,
		wrapHint: WrapHintBreak,
	})
}

// renderParagraph emits paragraph rows, wrapped per ctx.Wrap policy.
// Inline spans (bold/italic/code/link) are collected via collectInline.
func (w *mdWalker) renderParagraph(p *ast.Paragraph) {
	text, spans := w.collectInline(p)
	if text == "" {
		return
	}
	w.rows = append(w.rows, rowSpec{
		text:     text,
		style:    StyleNormal,
		spans:    spans,
		wrapHint: WrapHintContinue,
	})
}

// renderList walks an ordered or unordered list. indent depth is in
// half-step units (0 = root level, 1 = first nested, 2 = nested-nested).
// Ordered uses "N. " prefix, unordered uses "- " (CC standard, R3 fix).
func (w *mdWalker) renderList(l *ast.List, depth int) {
	pad := strings.Repeat("  ", depth)
	idx := 1
	for c := l.FirstChild(); c != nil; c = c.NextSibling() {
		item, ok := c.(*ast.ListItem)
		if !ok {
			continue
		}
		var prefix string
		if l.IsOrdered() {
			prefix = fmt.Sprintf("%s%d. ", pad, idx)
			idx++
		} else {
			prefix = pad + "- "
		}
		// ListItem may contain Paragraph + nested List etc.
		firstLine := true
		for ic := item.FirstChild(); ic != nil; ic = ic.NextSibling() {
			switch t := ic.(type) {
			case *ast.Paragraph, *ast.TextBlock:
				text, spans := w.collectInline(t)
				if text == "" {
					continue
				}
				if firstLine {
					// shift inline spans by len(prefix) so styling aligns.
					shifted := shiftSpans(spans, len(prefix))
					w.rows = append(w.rows, rowSpec{
						text:     prefix + text,
						style:    StyleNormal,
						spans:    shifted,
						wrapHint: WrapHintContinue,
					})
					firstLine = false
				} else {
					// Continuation line within same list item — align under text.
					indentCont := strings.Repeat(" ", len(prefix))
					w.rows = append(w.rows, rowSpec{
						text:     indentCont + text,
						style:    StyleNormal,
						spans:    shiftSpans(spans, len(indentCont)),
						wrapHint: WrapHintContinue,
					})
				}
			case *ast.List:
				w.renderList(t, depth+1)
			}
		}
	}
}

// renderBlockquote prefixes each contained line with "▎ " (U+258E)
// per CC figures (R3 baseline B-29).
func (w *mdWalker) renderBlockquote(b *ast.Blockquote) {
	startRow := len(w.rows)
	w.walkBlock(b)
	// Apply the rail prefix to every row emitted by the nested walk.
	prefix := "▎ "
	for i := startRow; i < len(w.rows); i++ {
		w.rows[i].text = prefix + w.rows[i].text
		w.rows[i].spans = shiftSpans(w.rows[i].spans, len(prefix))
		w.rows[i].style = StyleDimmed
	}
}

// renderFenceBlock delegates to spec-1.7 renderCodeBlock per D-6 R2 HIGH-1
// fixed signature (ctx, lang, body) → (buffer.Buffer, int).
func (w *mdWalker) renderFenceBlock(f *ast.FencedCodeBlock) {
	lang := string(f.Language(w.src))
	body := extractCodeBlockText(f, w.src)
	w.renderCodeViaSpec17(lang, body)
}

// renderIndentedCodeBlock handles 4-space indented code blocks; same
// delegate path as fenced, no lang.
func (w *mdWalker) renderIndentedCodeBlock(c *ast.CodeBlock) {
	body := extractCodeBlockText(c, w.src)
	w.renderCodeViaSpec17("", body)
}

// renderCodeViaSpec17 calls spec-1.7 renderCodeBlock and inlines the
// resulting rows into w.rows as raw styled rowSpecs. The intermediate
// Buffer is read cell-by-cell so the final buildBuffer write is uniform.
func (w *mdWalker) renderCodeViaSpec17(lang, body string) {
	codeBuf, _ := renderCodeBlock(w.ctx, lang, body)
	cols, rows := codeBuf.Size()
	for y := 0; y < rows; y++ {
		var b strings.Builder
		cells := make([]buffer.Cell, cols)
		for x := 0; x < cols; x++ {
			c := codeBuf.Cell(x, y)
			cells[x] = c
			if buffer.IsContinuation(c) {
				continue
			}
			if c.Ch == 0 {
				b.WriteRune(' ')
				continue
			}
			b.WriteRune(c.Ch)
		}
		w.rows = append(w.rows, rowSpec{
			text:     strings.TrimRight(b.String(), " "),
			cells:    cells,
			wrapHint: WrapHintKeep,
		})
	}
}

// renderHR emits a single dimmed "---" row (R3 baseline; not full-width
// U+2500). buildBuffer will style with StyleDimmed.
func (w *mdWalker) renderHR() {
	w.rows = append(w.rows, rowSpec{
		text:     "---",
		style:    StyleDimmed,
		wrapHint: WrapHintBreak,
	})
}

// renderTable emits a basic Box-drawing 2D grid. CC MarkdownTable is a
// separate component (B-29 MarkdownTable.tsx); opendbx Stage 1 ships
// minimal grid (header + rows) without alignment / footer / multi-line
// cells (❌-3).
func (w *mdWalker) renderTable(t *extast.Table) {
	var headers []string
	var dataRows [][]string

	for c := t.FirstChild(); c != nil; c = c.NextSibling() {
		switch tr := c.(type) {
		case *extast.TableHeader:
			for cell := tr.FirstChild(); cell != nil; cell = cell.NextSibling() {
				if tc, ok := cell.(*extast.TableCell); ok {
					headers = append(headers, w.extractInlineText(tc))
				}
			}
		case *extast.TableRow:
			var row []string
			for cell := tr.FirstChild(); cell != nil; cell = cell.NextSibling() {
				if tc, ok := cell.(*extast.TableCell); ok {
					row = append(row, w.extractInlineText(tc))
				}
			}
			dataRows = append(dataRows, row)
		}
	}

	if len(headers) == 0 && len(dataRows) == 0 {
		return
	}

	// Compute column widths.
	cols := len(headers)
	for _, r := range dataRows {
		if len(r) > cols {
			cols = len(r)
		}
	}
	widths := make([]int, cols)
	updateW := func(cells []string) {
		for i, c := range cells {
			if i >= cols {
				break
			}
			if l := width.Width(c); l > widths[i] {
				widths[i] = l
			}
		}
	}
	updateW(headers)
	for _, r := range dataRows {
		updateW(r)
	}

	formatRow := func(cells []string) string {
		var b strings.Builder
		b.WriteRune('│')
		for i := 0; i < cols; i++ {
			cell := ""
			if i < len(cells) {
				cell = cells[i]
			}
			pad := widths[i] - width.Width(cell)
			if pad < 0 {
				pad = 0
			}
			b.WriteRune(' ')
			b.WriteString(cell)
			b.WriteString(strings.Repeat(" ", pad))
			b.WriteString(" │")
		}
		return b.String()
	}
	separator := func(start, mid, end rune) string {
		var b strings.Builder
		b.WriteRune(start)
		for i := 0; i < cols; i++ {
			b.WriteString(strings.Repeat("─", widths[i]+2))
			if i < cols-1 {
				b.WriteRune(mid)
			}
		}
		b.WriteRune(end)
		return b.String()
	}

	if len(headers) > 0 {
		w.rows = append(w.rows,
			rowSpec{text: separator('┌', '┬', '┐'), style: StyleDimmed, wrapHint: WrapHintKeep},
			rowSpec{text: formatRow(headers), style: StyleBold, wrapHint: WrapHintKeep},
			rowSpec{text: separator('├', '┼', '┤'), style: StyleDimmed, wrapHint: WrapHintKeep},
		)
	} else {
		w.rows = append(w.rows,
			rowSpec{text: separator('┌', '┬', '┐'), style: StyleDimmed, wrapHint: WrapHintKeep},
		)
	}
	for _, r := range dataRows {
		w.rows = append(w.rows, rowSpec{
			text:     formatRow(r),
			style:    StyleNormal,
			wrapHint: WrapHintKeep,
		})
	}
	w.rows = append(w.rows,
		rowSpec{text: separator('└', '┴', '┘'), style: StyleDimmed, wrapHint: WrapHintKeep},
	)
}

// collectInline walks inline children of a block node and returns the
// concatenated text + styled spans for emphasis/strong/code/link.
// Strikethrough (~~text~~) stays literal per CC R3 baseline. Raw HTML
// nodes drop (CC formatToken empty for `html`).
func (w *mdWalker) collectInline(parent ast.Node) (string, []styledSpan) {
	var b strings.Builder
	var spans []styledSpan
	w.collectInlineInto(parent, &b, &spans)
	return b.String(), spans
}

// collectInlineInto recursively appends inline node text into the
// builder and styled span list.
func (w *mdWalker) collectInlineInto(parent ast.Node, b *strings.Builder, spans *[]styledSpan) {
	for c := parent.FirstChild(); c != nil; c = c.NextSibling() {
		switch n := c.(type) {
		case *ast.Text:
			b.WriteString(unescapeMarkdown(string(n.Segment.Value(w.src))))
			if n.HardLineBreak() || n.SoftLineBreak() {
				b.WriteRune(' ')
			}
		case *ast.String:
			b.WriteString(unescapeMarkdown(string(n.Value)))
		case *ast.Emphasis:
			start := b.Len()
			w.collectInlineInto(n, b, spans)
			end := b.Len()
			style := StyleItalic
			if n.Level == 2 {
				style = StyleBold
			}
			*spans = append(*spans, styledSpan{start: start, end: end, style: style})
		case *ast.CodeSpan:
			start := b.Len()
			b.WriteString("`")
			for ic := n.FirstChild(); ic != nil; ic = ic.NextSibling() {
				if t, ok := ic.(*ast.Text); ok {
					b.Write(t.Segment.Value(w.src))
				}
			}
			b.WriteString("`")
			end := b.Len()
			*spans = append(*spans, styledSpan{start: start, end: end, style: StyleCode})
		case *ast.Link:
			start := b.Len()
			w.collectInlineInto(n, b, spans)
			b.WriteString(" (")
			b.Write(n.Destination)
			b.WriteString(")")
			end := b.Len()
			*spans = append(*spans, styledSpan{start: start, end: end, style: StyleLink})
		case *ast.AutoLink:
			start := b.Len()
			b.Write(n.URL(w.src))
			end := b.Len()
			*spans = append(*spans, styledSpan{start: start, end: end, style: StyleLink})
		case *ast.RawHTML, *ast.HTMLBlock:
			// ❌-4 + I-5: drop raw HTML per CC formatToken empty.
			continue
		default:
			// Unknown inline node — descend in case it has text children.
			w.collectInlineInto(c, b, spans)
		}
	}
}

// extractInlineText returns the plain text of inline children (no
// styling). Used for heading + table cell short-text contexts.
func (w *mdWalker) extractInlineText(parent ast.Node) string {
	var b strings.Builder
	for c := parent.FirstChild(); c != nil; c = c.NextSibling() {
		switch n := c.(type) {
		case *ast.Text:
			b.WriteString(unescapeMarkdown(string(n.Segment.Value(w.src))))
		case *ast.String:
			b.WriteString(unescapeMarkdown(string(n.Value)))
		default:
			b.WriteString(w.extractInlineText(c))
		}
	}
	return b.String()
}

// unescapeMarkdown processes CommonMark backslash escapes. The spec
// defines an escapable set; any `\X` where X is in the set produces X
// literally. Anything else preserves the backslash.
//
// Set per CommonMark 0.31: ` ! " # $ % & ' ( ) * + , - . / : ; < = > ? @ [ \ ] ^ _ ` { | } ~
func unescapeMarkdown(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) && isMarkdownEscapable(s[i+1]) {
			b.WriteByte(s[i+1])
			i++
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// isMarkdownEscapable reports whether a byte is in the CommonMark
// backslash-escape set.
func isMarkdownEscapable(c byte) bool {
	switch c {
	case '!', '"', '#', '$', '%', '&', '\'', '(', ')', '*', '+', ',', '-',
		'.', '/', ':', ';', '<', '=', '>', '?', '@', '[', '\\', ']', '^',
		'_', '`', '{', '|', '}', '~':
		return true
	}
	return false
}

// extractCodeBlockText returns the concatenated text content of a
// code-block node's child Lines.
func extractCodeBlockText(n ast.Node, src []byte) string {
	var b strings.Builder
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		b.Write(seg.Value(src))
	}
	return b.String()
}

// shiftSpans returns a copy of spans with each (start, end) shifted by
// delta bytes. Used when a row prefix (e.g., "- ", "▎ ") is prepended
// so inline styles continue to align with their text.
func shiftSpans(spans []styledSpan, delta int) []styledSpan {
	if len(spans) == 0 || delta == 0 {
		return spans
	}
	out := make([]styledSpan, len(spans))
	for i, s := range spans {
		out[i] = styledSpan{start: s.start + delta, end: s.end + delta, style: s.style}
	}
	return out
}

// buildBuffer materializes []rowSpec → buffer.Buffer (HIGH-2 contract).
// Applies ctx.Wrap policy then writes each row's text + styled spans
// into a buffer.Grid.
func (w *mdWalker) buildBuffer() buffer.Buffer {
	if len(w.rows) == 0 {
		return measureOnlyBuf(w.ctx.Cols, 0)
	}

	// Expand each rowSpec into one or more physical rows via wrap().
	var expanded []rowSpec
	for _, r := range w.rows {
		if r.wrapHint == WrapHintKeep {
			// Don't wrap; let it overflow horizontally (table/fence).
			expanded = append(expanded, r)
			continue
		}
		lines := wrap(r.text, w.ctx.Cols, w.ctx.Wrap)
		if len(lines) == 0 {
			expanded = append(expanded, r)
			continue
		}
		for i, line := range lines {
			// Only the first wrapped line keeps the original spans;
			// subsequent lines lose span styling (spec-1.11 MVP — full
			// span tracking through wrap is spec-1.21 follow-on).
			var spans []styledSpan
			if i == 0 {
				spans = r.spans
			}
			expanded = append(expanded, rowSpec{
				text:     line,
				style:    r.style,
				spans:    spans,
				wrapHint: r.wrapHint,
			})
		}
	}

	rows := len(expanded)
	if rows == 0 {
		return measureOnlyBuf(w.ctx.Cols, 0)
	}
	if w.ctx.MeasureOnly {
		return measureOnlyBuf(w.ctx.Cols, rows)
	}
	grid, err := buffer.NewGrid(w.ctx.Cols, rows)
	if err != nil {
		return measureOnlyBuf(w.ctx.Cols, rows)
	}
	for y, r := range expanded {
		if len(r.cells) > 0 {
			writeRawCells(grid, y, r.cells, w.ctx.Cols)
			continue
		}
		baseStyle := w.theme.Style(r.style)
		writeTextRow(grid, 0, y, r.text, baseStyle, w.ctx.Cols)
		// Apply styled spans on top of base style.
		for _, sp := range r.spans {
			spanStyle := w.theme.Style(sp.style)
			applySpanStyle(grid, y, r.text, sp.start, sp.end, spanStyle, w.ctx.Cols)
		}
	}
	return grid
}

// writeRawCells copies a delegated Buffer row while preserving per-cell style.
// Continuation cells are skipped because SetCell writes them from the wide main
// cell; writing the continuation directly would clear the main cell.
func writeRawCells(grid *buffer.Grid, y int, cells []buffer.Cell, cols int) {
	for x, c := range cells {
		if x >= cols {
			break
		}
		if buffer.IsContinuation(c) {
			continue
		}
		if c.Ch == 0 && c.St == (style.Style{}) {
			continue
		}
		grid.SetCell(x, y, c)
	}
}

// applySpanStyle overlays a span's style onto cells within [start, end)
// byte offsets of the given text. Walks runes parallel to writeTextRow.
func applySpanStyle(grid *buffer.Grid, y int, text string, byteStart, byteEnd int, s style.Style, cols int) {
	x := 0
	for i := 0; i < len(text) && x < cols; {
		r, size := utf8.DecodeRuneInString(text[i:])
		rw := width.RuneWidth(r)
		if rw <= 0 {
			i += size
			continue
		}
		if x+rw > cols {
			break
		}
		if i >= byteStart && i < byteEnd {
			cell := grid.Cell(x, y)
			cell.St = s
			grid.SetCell(x, y, cell)
		}
		x += rw
		i += size
	}
}
