// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File compact.go — production CompactSummary block (spec-1.10 D-1/D-2).
// Aggregates N grouped Read/Search/List ToolUse into 1 collapsed visual
// row per CC `CollapsedReadSearchGroup` (B-21 collapseReadSearch.ts:
// 663-720) visual-render subset. Per Q9 ★A 架构 boundary: expanded
// mode 由 caller (spec-1.21) 渲染原 block.ToolUse + ToolResult,
// CompactSummary 自身 0 rows 让位. React-internal grouping 字段 (Messages
// / DisplayMessage / UUID / Timestamp / MCPCallCount / MCPServerNames /
// bash/git/fullscreen meta per R4 MED-2) 不入 block struct.

package block

import (
	"fmt"
	"strings"

	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
)

// CompactSummary is the spec-1.10 10th block type — visual-render subset
// of CC CollapsedReadSearchGroup (B-21). No Elapsed field (R4 MED-4:
// fullscreen bash elapsed / formatDuration deferred per ❌-7).
type CompactSummary struct {
	SearchCount            int
	ReadCount              int
	ListCount              int
	ReplCount              int
	MemorySearchCount      int
	MemoryReadCount        int
	MemoryWriteCount       int
	NonMemoryReadFilePaths []string // CC nonMemReadFilePaths filtered subset
	SearchArgs             []string
	LatestDisplayHint      string
	ToolUseIDs             []string // back-ref to peer block.ToolUse.IDs; caller-side join
	IsActive               bool     // CC isActiveGroup: present/past tense + hint row gate
}

// NewCompactSummary returns a zero-value CompactSummary; caller fills
// fields per its aggregation (spec-1.21 diagnose-loop).
func NewCompactSummary() CompactSummary { return CompactSummary{} }

// Render produces a Buffer per spec-1.10 D-2 (R2/R4 dispatch).
//
// Dispatch (R2 MED-2 用户拍板 IsTranscript 跟 CC verbose 一致):
//   - ctx.IsTranscript=false && ctx.Verbose=false → 1-row collapsed
//     summary + optional hint row (IsActive=true + LatestDisplayHint
//     non-empty per R2 HIGH-2 + B-26)
//   - ctx.IsTranscript=true OR ctx.Verbose=true → 0 rows (expanded
//     mode; caller renders 原 ToolUse + ToolResult per Q9 ★A)
//
// Collapsed text format (R2 CRIT-2 + R4 MED-3):
//
//	"<indicator> Part1, Part2, …"
//
// Part order = CC CollapsedReadSearchContent.tsx :294-415 visible
// component order (B-22 authoritative per R4 MED-3): non-memory first
// (Search/Read/List), then memory parts (Recalled/Searched/Wrote).
// Separator `, ` (not `·`); plural-aware (file/files,
// directory/directories, pattern/patterns, memory/memories); empty
// counts omitted; verb tense by IsActive (present/past); first part
// uppercase verb, subsequent parts lowercase (CC component pattern).
// No mcp/bash/git/fullscreen parts (R4 MED-2 scope).
// No elapsed suffix on summary line (R4 MED-4).
//
// MeasureOnly fast path: returns Buffer with correct rows count, no
// cell writes (spec-1.7 D-1 contract).
func (c CompactSummary) Render(ctx Context) (buffer.Buffer, error) {
	if ctx.Cols <= 0 {
		return measureOnlyBuf(0, 0), nil
	}
	// Expanded mode → 0 rows (caller renders原 ToolUse/ToolResult).
	if ctx.IsTranscript || ctx.Verbose {
		return measureOnlyBuf(ctx.Cols, 0), nil
	}

	theme := themeOrDefault(ctx.Theme)
	parts := c.collectParts()
	if len(parts) == 0 && c.LatestDisplayHint == "" {
		// Degenerate empty group → 0 rows.
		return measureOnlyBuf(ctx.Cols, 0), nil
	}

	rows := make([]blockRow, 0, 2)
	if len(parts) > 0 {
		ind := compactIndicator(ToolGroupCategoryDefault)
		summary := fmt.Sprintf("%c %s", ind.Rune, strings.Join(parts, ", "))
		rows = append(rows, blockRow{text: summary, style: ind.Style})
	}
	if c.IsActive && c.LatestDisplayHint != "" {
		hintText := getDisplayPath(c.LatestDisplayHint)
		rows = append(rows, blockRow{text: "↳ " + hintText, style: StyleDimmed})
	}

	renderRows := expandBlockRows(ctx, rows, ExpandOptions{SplitNewlines: false})
	if len(renderRows) == 0 {
		return measureOnlyBuf(ctx.Cols, 0), nil
	}
	if ctx.MeasureOnly {
		return measureOnlyBuf(ctx.Cols, len(renderRows)), nil
	}
	buf, err := buffer.NewGrid(ctx.Cols, len(renderRows))
	if err != nil {
		return measureOnlyBuf(ctx.Cols, len(renderRows)), nil
	}
	for y, r := range renderRows {
		writeTextRow(buf, 0, y, r.text, theme.Style(r.style), ctx.Cols)
	}
	return buf, nil
}

// collectParts builds the visible parts per CC component order:
// non-memory first (Search/Read/List), then memory (Recalled / Searched
// memories / Wrote). First part uses uppercase verb; subsequent parts
// use lowercase (CC visible pattern per R4 MED-3 B-22).
func (c CompactSummary) collectParts() []string {
	var parts []string

	addPart := func(verb, lowerVerb, noun string, n int) {
		if n <= 0 {
			return
		}
		v := lowerVerb
		if len(parts) == 0 {
			v = verb
		}
		parts = append(parts, fmt.Sprintf("%s %d %s", v, n, plural(n, noun, noun+"s")))
	}

	// Non-memory parts first (CC B-22 :294-415 visible order; R4 MED-3).
	// Search uses "patterns" noun; Read uses "files"; List uses
	// "directory/directories".
	if c.SearchCount > 0 {
		searchVerb, searchVerbLower := tenseVerb(c.IsActive, "Searching for", "Searched for", "searching for", "searched for")
		v := searchVerbLower
		if len(parts) == 0 {
			v = searchVerb
		}
		parts = append(parts, fmt.Sprintf("%s %d %s", v, c.SearchCount, plural(c.SearchCount, "pattern", "patterns")))
	}
	if c.ReadCount > 0 {
		readVerb, readVerbLower := tenseVerb(c.IsActive, "Reading", "Read", "reading", "read")
		addTenseCountPart(&parts, readVerb, readVerbLower, "file", "files", c.ReadCount)
	}
	if c.ListCount > 0 {
		listVerb, listVerbLower := tenseVerb(c.IsActive, "Listing", "Listed", "listing", "listed")
		addListPart(&parts, listVerb, listVerbLower, c.ListCount)
	}
	// REPL deliberately not rendered (replCount=0 in spec-1.10; CC :712).

	// Memory parts (CC B-22 :294-415 after non-mem).
	if c.MemoryReadCount > 0 {
		recalledVerb, recalledVerbLower := tenseVerb(c.IsActive, "Recalling", "Recalled", "recalling", "recalled")
		addTenseCountPart(&parts, recalledVerb, recalledVerbLower, "memory", "memories", c.MemoryReadCount)
	}
	if c.MemorySearchCount > 0 {
		mSearchVerb, mSearchVerbLower := tenseVerb(c.IsActive, "Searching", "Searched", "searching", "searched")
		// "Searched memories" (no count word per CC pattern)
		v := mSearchVerbLower
		if len(parts) == 0 {
			v = mSearchVerb
		}
		parts = append(parts, fmt.Sprintf("%s memories", v))
	}
	if c.MemoryWriteCount > 0 {
		writeVerb, writeVerbLower := tenseVerb(c.IsActive, "Writing", "Wrote", "writing", "wrote")
		addTenseCountPart(&parts, writeVerb, writeVerbLower, "memory", "memories", c.MemoryWriteCount)
	}

	_ = addPart // future use if uniform path needed
	return parts
}

// addTenseCountPart appends a "<verb> <N> <noun(plural)>" part with
// first-part uppercase / subsequent-part lowercase verb selection.
func addTenseCountPart(parts *[]string, verb, verbLower, singular, pluralForm string, n int) {
	v := verbLower
	if len(*parts) == 0 {
		v = verb
	}
	*parts = append(*parts, fmt.Sprintf("%s %d %s", v, n, plural(n, singular, pluralForm)))
}

// addListPart appends the listing part with directory/directories noun.
func addListPart(parts *[]string, verb, verbLower string, n int) {
	v := verbLower
	if len(*parts) == 0 {
		v = verb
	}
	noun := "directory"
	if n != 1 {
		noun = "directories"
	}
	*parts = append(*parts, fmt.Sprintf("%s %d %s", v, n, noun))
}

// tenseVerb returns (firstPartVerb, subsequentPartVerb) given IsActive.
// IsActive=true → present-tense verbs; IsActive=false → past-tense verbs.
// First-part receives uppercase; non-first receives lowercase (CC :347-380
// visible pattern).
func tenseVerb(isActive bool, presentUpper, pastUpper, presentLower, pastLower string) (string, string) {
	if isActive {
		return presentUpper, presentLower
	}
	return pastUpper, pastLower
}

// plural returns singular or pluralForm based on n.
func plural(n int, singular, pluralForm string) string {
	if n == 1 {
		return singular
	}
	return pluralForm
}
