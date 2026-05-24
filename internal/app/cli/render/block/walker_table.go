// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File walker_table.go — spec-1.13 R2 HIGH-2 extraction.
//
// Hosts the markdown table render family extracted from walker.go to bring
// that file under the 800-line hard limit (CLAUDE.md rule 12). No behaviour
// change vs. walker.go pre-extraction; functions retain *mdWalker receiver
// where present so all internal state access stays identical.
//
// Original residence: walker.go lines 352-476 (renderTable + four helpers).

package block

import (
	"strings"

	extast "github.com/yuin/goldmark/extension/ast"

	"github.com/sqlrush/opendbx/internal/app/cli/render/width"
)

// renderTable emits a basic Box-drawing 2D grid. CC MarkdownTable is a
// separate component (B-29 MarkdownTable.tsx); opendbx Stage 1 ships
// minimal grid (header + rows) without alignment / footer / multi-line
// cells (❌-3).
//
// R6 HIGH-2: extracted tableFormatRow + tableSeparator helpers below
// to satisfy rule 12 ≤100 line function limit.
func (w *mdWalker) renderTable(t *extast.Table) {
	headers, dataRows := tableCollectCells(t, w)
	if len(headers) == 0 && len(dataRows) == 0 {
		return
	}
	cols, widths := tableComputeWidths(headers, dataRows)

	if len(headers) > 0 {
		w.rows = append(w.rows,
			rowSpec{text: tableSeparator('┌', '┬', '┐', widths, cols), style: StyleDimmed, wrapHint: WrapHintKeep},
			rowSpec{text: tableFormatRow(headers, widths, cols), style: StyleBold, wrapHint: WrapHintKeep},
			rowSpec{text: tableSeparator('├', '┼', '┤', widths, cols), style: StyleDimmed, wrapHint: WrapHintKeep},
		)
	} else {
		w.rows = append(w.rows,
			rowSpec{text: tableSeparator('┌', '┬', '┐', widths, cols), style: StyleDimmed, wrapHint: WrapHintKeep},
		)
	}
	for _, r := range dataRows {
		w.rows = append(w.rows, rowSpec{
			text:     tableFormatRow(r, widths, cols),
			style:    StyleNormal,
			wrapHint: WrapHintKeep,
		})
	}
	w.rows = append(w.rows,
		rowSpec{text: tableSeparator('└', '┴', '┘', widths, cols), style: StyleDimmed, wrapHint: WrapHintKeep},
	)
}

// tableCollectCells extracts headers + data rows from a goldmark table AST.
// R6 HIGH-2 extracted from renderTable.
func tableCollectCells(t *extast.Table, w *mdWalker) (headers []string, dataRows [][]string) {
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
	return
}

// tableComputeWidths returns the column count and per-column max display
// width across headers + data rows. R6 HIGH-2 extracted from renderTable.
func tableComputeWidths(headers []string, dataRows [][]string) (cols int, widths []int) {
	cols = len(headers)
	for _, r := range dataRows {
		if len(r) > cols {
			cols = len(r)
		}
	}
	widths = make([]int, cols)
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
	return
}

// tableFormatRow renders a single data/header row using the precomputed
// column widths. R6 HIGH-2 extracted from renderTable closure.
func tableFormatRow(cells []string, widths []int, cols int) string {
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

// tableSeparator renders a horizontal separator row using box-drawing
// glyphs (┌┬┐ top, ├┼┤ mid, └┴┘ bottom). R6 HIGH-2 extracted from
// renderTable closure.
func tableSeparator(start, mid, end rune, widths []int, cols int) string {
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
