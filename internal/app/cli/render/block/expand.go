// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File expand.go — block-internal common row+wrap helper (spec-1.10 D-5
// consolidation per spec-1.9b Q6 ★A "spec-1.10 时 consolidate"). Replaces
// per-spec `toolUseRow` / `toolResultRow` duplication with a single
// `blockRow` package type; `expandToolUseRows` (spec-1.9) and
// `expandToolResultRows` (spec-1.9b) are thin wrappers calling
// `expandBlockRows` w/ `ExpandOptions{SplitNewlines: false/true}` per
// spec-1.10 R2 MED-1 split-semantics gap fix.

package block

import "strings"

// blockRow is the unified logical row type: text content + style kind.
// spec-1.9 `toolUseRow` and spec-1.9b `toolResultRow` are type aliases
// of blockRow (rule 21 thin-wrapper preserve signatures).
type blockRow struct {
	text  string
	style StyleKind
}

// ExpandOptions controls pre-processing behavior of expandBlockRows.
//
// spec-1.10 R2 MED-1 absorb: spec-1.9 expandToolUseRows wrapped row.text
// directly through wrap(); spec-1.9b expandToolResultRows additionally
// pre-split row.text on "\n" before wrapping each segment. These two
// behaviors are NOT loop-equivalent (an embedded newline produces
// different row counts), so the consolidation must allow callers to
// choose.
type ExpandOptions struct {
	// SplitNewlines, when true, splits each row.text on "\n" before
	// invoking wrap() on each segment; produces multiple output rows
	// per input row with newlines. When false, row.text is passed to
	// wrap() as-is (newlines preserved in wrapped output per wrap()
	// semantics).
	SplitNewlines bool
}

// expandBlockRows applies ctx.Wrap policy to each input row, optionally
// pre-splitting on "\n" per opts.SplitNewlines. Empty wrap() output for
// a row preserves the original row to avoid silently dropping content.
// spec-1.10 D-5 consolidation.
func expandBlockRows(ctx Context, rows []blockRow, opts ExpandOptions) []blockRow {
	out := make([]blockRow, 0, len(rows))
	for _, r := range rows {
		if opts.SplitNewlines {
			for _, segment := range strings.Split(r.text, "\n") {
				lines := wrap(segment, ctx.Cols, ctx.Wrap)
				if len(lines) == 0 {
					lines = []string{segment}
				}
				for _, line := range lines {
					out = append(out, blockRow{text: line, style: r.style})
				}
			}
			continue
		}
		lines := wrap(r.text, ctx.Cols, ctx.Wrap)
		if len(lines) == 0 {
			out = append(out, r)
			continue
		}
		for _, line := range lines {
			out = append(out, blockRow{text: line, style: r.style})
		}
	}
	return out
}
