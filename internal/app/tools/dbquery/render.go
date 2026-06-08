// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File render.go — aligned text-table rendering of a db.QueryResult for
// the tool result content (spec-2.3a D-3 / Q5).
//
// This is WIRE text for the LLM (and /report), NOT terminal UI — column
// alignment uses rune counts, not East-Asian display width (that is the
// § 3.9 UI table concern, explicitly out of scope here, ❌-9). The total
// output is capped at 16KiB with a UTF-8-safe cut so a multibyte rune at
// the boundary is never split (spec-2.3a codex MED-1; the 64KiB→16KiB cap
// also removes the conflict with spec-2.3 R-7's body warning).

package dbquery

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/sqlrush/opendbx/internal/domain/db"
)

// maxOutputBytes caps the rendered table (spec-2.3a Q6, user decision
// 64KiB→16KiB; ≈4K tokens, within the 规则 19 context budget).
const maxOutputBytes = 16 << 10

// truncSuffix is appended when the rendered table exceeds maxOutputBytes.
const truncSuffix = "\n…(output truncated at 16KiB)"

// renderTable renders the result as an aligned text table plus a footer.
// Empty column set → "(0 rows)".
func renderTable(res db.QueryResult) string {
	if len(res.Columns) == 0 {
		return "(0 rows)"
	}

	widths := make([]int, len(res.Columns))
	for i, c := range res.Columns {
		widths[i] = runeLen(c)
	}
	for _, row := range res.Rows {
		for i, cell := range row {
			if w := runeLen(cell); i < len(widths) && w > widths[i] {
				widths[i] = w
			}
		}
	}

	var b strings.Builder
	writeRow(&b, res.Columns, widths)
	seps := make([]string, len(widths))
	for i, w := range widths {
		seps[i] = strings.Repeat("-", w)
	}
	writeRow(&b, seps, widths)
	for _, row := range res.Rows {
		writeRow(&b, row, widths)
	}
	b.WriteString(footer(res))

	return capUTF8(b.String(), maxOutputBytes)
}

// writeRow writes one left-aligned row. The trailing column is not padded
// (no dangling spaces — keeps goldens tight). Columns are separated by two
// spaces.
func writeRow(b *strings.Builder, cells []string, widths []int) {
	for i, cell := range cells {
		if i > 0 {
			b.WriteString("  ")
		}
		b.WriteString(cell)
		if i < len(cells)-1 {
			if pad := widths[i] - runeLen(cell); pad > 0 {
				b.WriteString(strings.Repeat(" ", pad))
			}
		}
	}
	b.WriteByte('\n')
}

// footer is the row-count line with truncation annotations (spec-2.3a Q5).
func footer(res db.QueryResult) string {
	n := len(res.Rows)
	var s string
	if res.RowsTruncated {
		s = fmt.Sprintf("(first %d rows; result truncated)", n)
	} else {
		s = fmt.Sprintf("(%d rows)", n)
	}
	if res.CellsTruncated {
		s += " (some cells truncated)"
	}
	return s
}

// capUTF8 truncates s to at most max bytes, never splitting a rune, and
// appends a visible note when it cuts (spec-2.3a codex MED-1).
func capUTF8(s string, max int) string {
	if len(s) <= max {
		return s
	}
	budget := max - len(truncSuffix)
	if budget < 0 {
		budget = 0
	}
	cut := s[:budget]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut + truncSuffix
}

func runeLen(s string) int { return utf8.RuneCountInString(s) }
