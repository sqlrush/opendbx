// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File toolresult.go — production ToolResult block (spec-1.9b D-1).
// Bifurcated from spec-1.9 ToolUse per spec-1.9 R2 HIGH-1. 4-enum state
// (Success / Error / Rejected / Canceled) derived per CC priority
// UserToolResultMessage.tsx:40-104 (B-12); NewToolResult constructs at
// call site with peer-ToolUse.Name copied via tool_use_id join (R3 HIGH-3).

package block

import (
	"fmt"
	"strings"

	"github.com/sqlrush/opendbx/internal/app/cli/render/block/adapter"
	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
)

// ToolResultState enumerates the 4 result paths per CC
// UserToolResultMessage.tsx:40-104 if-chain (B-12; spec-1.9b R2 LOW-1
// terminology — priority not switch).
type ToolResultState int

// Lifecycle constants for ToolResultState (spec-1.9b D-1).
const (
	ResultSuccess ToolResultState = iota
	ResultError
	ResultRejected
	ResultCanceled
)

// CC SDK literal sentence constants per B-16 (claude-code-source-code
// src/utils/messages.ts:207-213). R3 HIGH-1 locks the full sentences
// (R2 only stored opening fragments). INTERRUPT uses EXACT EQUALITY
// not startsWith (CC UserToolResultMessage.tsx:50 second half).
const (
	// cancelMessagePrefix is the full CC CANCEL_MESSAGE sentence.
	// CC matches via `content.startsWith(CANCEL_MESSAGE)` (CC :40).
	cancelMessagePrefix = "The user doesn't want to take this action right now. STOP what you are doing and wait for the user to tell you how to proceed."

	// rejectMessagePrefix is the full CC REJECT_MESSAGE sentence.
	// CC matches via `content.startsWith(REJECT_MESSAGE)` (CC :50 first half).
	rejectMessagePrefix = "The user doesn't want to proceed with this tool use. The tool use was rejected (eg. if it was a file edit, the new_string was NOT written to the file). STOP what you are doing and wait for the user to tell you how to proceed."

	// interruptMessageExact is the EXACT CC INTERRUPT_MESSAGE_FOR_TOOL_USE
	// string. CC uses `content === INTERRUPT_MESSAGE_FOR_TOOL_USE` (CC :50
	// second half, equality not prefix; R2 HIGH-1 + R3 HIGH-1).
	interruptMessageExact = "[Request interrupted by user for tool use]"

	// interruptedByUserText is the CC InterruptedByUser fixed display string
	// used for both Canceled state and Rejected fallback (no RejectedRenderer).
	// R3 HIGH-2: replaces R2 "Tool canceled by user" placeholder.
	interruptedByUserText = "Interrupted · What should Claude do instead?"
)

// ToolResult is the spec-1.9b new production block type (9th block type;
// spec-0.13 D-3 originally only listed 8 stubs without ToolResult). Maps
// to Anthropic SDK `ToolResultBlockParam` (B-11 UserToolResultMessage.tsx:
// 12-22):
//
//   - ToolUseID: back-reference to spec-1.9 ToolUse.ID (param.tool_use_id);
//     caller-side join, block does not validate.
//   - Content:   raw content from param.content (success string/structured;
//     error string; canceled string with CANCEL_MESSAGE prefix; etc.).
//   - IsError:   param.is_error.
//   - ToolName:  required adapter dispatch key copied from peer ToolUse.Name
//     after tool_use_id join. R3 HIGH-3: if peer ToolUse is missing,
//     upstream (spec-1.21) MUST skip rendering this ToolResult to match
//     CC UserToolResultMessage.tsx:36-39 `return null`; empty ToolName is
//     only allowed in explicit Generic fallback tests.
//   - State:     derived 4-enum set by NewToolResult; caller may override
//     only for explicit recovery / diagnostic paths.
type ToolResult struct {
	ToolUseID string
	Content   any
	IsError   bool
	ToolName  string
	State     ToolResultState
}

// NewToolResult constructs a ToolResult and derives State per CC if-chain
// priority order (B-12; R3 HIGH-3 adds ToolName ctor parameter):
//
//  1. Content (string) startsWith CANCEL_MESSAGE   → ResultCanceled
//  2. Content (string) startsWith REJECT_MESSAGE   → ResultRejected
//     OR Content (string) == INTERRUPT_MESSAGE     → ResultRejected
//  3. IsError == true (and not Canceled/Rejected)  → ResultError
//  4. default                                       → ResultSuccess
//
// Caller MUST pass toolName copied from the peer ToolUse.Name after
// tool_use_id join. Caller MAY pass an empty toolName only for explicit
// Generic fallback / unknown-tool tests; production upstream must skip
// the render entirely when no peer ToolUse exists (R3 HIGH-3 / CC null
// return).
func NewToolResult(toolUseID, toolName string, content any, isError bool) ToolResult {
	tr := ToolResult{
		ToolUseID: toolUseID,
		ToolName:  toolName,
		Content:   content,
		IsError:   isError,
	}
	tr.State = deriveResultState(content, isError)
	return tr
}

// deriveResultState applies CC priority order to (content, isError) per
// B-12. Non-string content cannot match the sentence-prefix rules, so
// it falls through to IsError / Success.
func deriveResultState(content any, isError bool) ToolResultState {
	if s, ok := content.(string); ok {
		switch {
		case strings.HasPrefix(s, cancelMessagePrefix):
			return ResultCanceled
		case strings.HasPrefix(s, rejectMessagePrefix):
			return ResultRejected
		case s == interruptMessageExact:
			return ResultRejected
		}
	}
	if isError {
		return ResultError
	}
	return ResultSuccess
}

// toolResultRow is a logical row + style pair for ToolResult.Render
// (local helper; ToolUse uses its own toolUseRow — kept separate to
// preserve type-distinctness per spec-1.9b Q6 ★A no-cross-call rule).
type toolResultRow struct {
	text  string
	style StyleKind
}

// Render produces a Buffer per spec-1.9b D-4 (R3 propagated layout).
//
// Per-state layout:
//   - ResultSuccess:  indicator + adapter.ResultRenderer.RenderResult if
//     non-empty; else NO ROWS (CC omits result output when adapter is
//     absent / returns null; spec-1.9b R3 MED-1).
//   - ResultError:    indicator StyleWarning + adapter.ErrorResultRenderer
//     .RenderErrorResult; else CC FallbackToolUseErrorMessage-equivalent
//     formatting of Content.
//   - ResultRejected: indicator StyleWarning + adapter.RejectedRenderer
//     .RenderRejected; else fixed CC InterruptedByUser text (R3 HIGH-2).
//   - ResultCanceled: indicator StyleDimmed + fixed CC InterruptedByUser
//     text (no adapter override per Q8 ★A; R3 HIGH-2 text lock).
//
// Adapter dispatch: adapter.Default.Lookup(t.ToolName); fallback Generic
// (with synthetic _name key) only for explicit Generic / unknown-tool
// renderings after upstream tool_use_id join (caller's responsibility
// to skip when peer ToolUse missing per R3 HIGH-3).
//
// All rows expanded through ctx.Wrap LOCAL loop (rule 21 — do NOT call
// spec-1.9 expandToolUseRows since spec-1.9 FROZEN; consolidate at
// spec-1.10 compact-block per Q6 ★A).
//
// MeasureOnly fast path: returns Buffer with correct Size().rows but
// cells no-op (spec-1.7 D-1 contract).
func (t ToolResult) Render(ctx Context) (buffer.Buffer, error) {
	if ctx.Cols <= 0 {
		return measureOnlyBuf(0, 0), nil
	}
	theme := themeOrDefault(ctx.Theme)
	rdr := adapter.Default.Lookup(t.ToolName)
	adapterCtx := adapter.Context{Verbose: ctx.Verbose, Cols: ctx.Cols}

	rows, err := t.collectRows(rdr, adapterCtx)
	if err != nil {
		return resultErrorPlaceholder(ctx, theme, t.ToolName)
	}

	renderRows := expandToolResultRows(ctx, rows)
	if len(renderRows) == 0 {
		// CC null-return path: Success with no adapter rows → 0 rows.
		return measureOnlyBuf(ctx.Cols, 0), nil
	}
	if ctx.MeasureOnly {
		return measureOnlyBuf(ctx.Cols, len(renderRows)), nil
	}
	buf, berr := buffer.NewGrid(ctx.Cols, len(renderRows))
	if berr != nil {
		return measureOnlyBuf(ctx.Cols, len(renderRows)), nil
	}
	for y, r := range renderRows {
		writeTextRow(buf, 0, y, r.text, theme.Style(r.style), ctx.Cols)
	}
	return buf, nil
}

// collectRows builds the logical rows per state. Returns the rows and
// any adapter error encountered. Empty result-success → no rows (R3 MED-1).
func (t ToolResult) collectRows(rdr adapter.HeaderRenderer, actx adapter.Context) ([]toolResultRow, error) {
	ind := resultIndicator(t.State)

	switch t.State {
	case ResultSuccess:
		text, err := callResultRenderer(rdr, t.Content, actx)
		if err != nil {
			return nil, err
		}
		if text == "" {
			return nil, nil // CC null-return path
		}
		return []toolResultRow{{
			text:  fmt.Sprintf("%c %s", ind.Rune, text),
			style: StyleNormal,
		}}, nil

	case ResultError:
		text, err := callErrorResultRenderer(rdr, t.Content, actx)
		if err != nil {
			return nil, err
		}
		if text == "" {
			text = adapter.FormatFallbackToolUseError(t.Content, actx)
		}
		return []toolResultRow{{
			text:  fmt.Sprintf("%c %s", ind.Rune, text),
			style: ind.Style,
		}}, nil

	case ResultRejected:
		text, err := callRejectedRenderer(rdr, t.Content, actx)
		if err != nil {
			return nil, err
		}
		if text == "" {
			text = interruptedByUserText
		}
		return []toolResultRow{{
			text:  fmt.Sprintf("%c %s", ind.Rune, text),
			style: ind.Style,
		}}, nil

	case ResultCanceled:
		// CC fixed text; no adapter override (Q8 ★A; R3 HIGH-2).
		return []toolResultRow{{
			text:  fmt.Sprintf("%c %s", ind.Rune, interruptedByUserText),
			style: ind.Style,
		}}, nil
	}
	return nil, nil
}

func callResultRenderer(rdr adapter.HeaderRenderer, content any, ctx adapter.Context) (string, error) {
	if rr, ok := rdr.(adapter.ResultRenderer); ok {
		return rr.RenderResult(content, ctx)
	}
	// No registered adapter or registered adapter doesn't implement
	// ResultRenderer: Generic fallback applies only when content has
	// actual data (spec-1.9b D-4: "fallback Generic only for explicit
	// Generic/unknown-tool rendering"). Empty / nil content triggers
	// R3 MED-1 null-row path (CC `renderToolResultMessage` absent).
	//
	// spec-1.9b T-9 LOW-1 note: a registered HeaderRenderer that does NOT
	// implement ResultRenderer (e.g., future ToolUse-only adapter) also
	// falls through to Generic here. spec-1.21 may revisit if a tool
	// needs explicit "result side absent" semantics — current behavior is
	// the same as unregistered (Generic compact text).
	if isEmptyContent(content) {
		return "", nil
	}
	return adapter.Generic{}.RenderResult(content, ctx)
}

func callErrorResultRenderer(rdr adapter.HeaderRenderer, content any, ctx adapter.Context) (string, error) {
	if er, ok := rdr.(adapter.ErrorResultRenderer); ok {
		return er.RenderErrorResult(content, ctx)
	}
	return "", nil
}

func callRejectedRenderer(rdr adapter.HeaderRenderer, content any, ctx adapter.Context) (string, error) {
	if rj, ok := rdr.(adapter.RejectedRenderer); ok {
		return rj.RenderRejected(content, ctx)
	}
	return "", nil
}

// isEmptyContent reports whether content is effectively empty (nil,
// empty string, or empty []byte). Used by callResultRenderer to gate
// Generic fallback per R3 MED-1.
func isEmptyContent(content any) bool {
	switch v := content.(type) {
	case nil:
		return true
	case string:
		return v == ""
	case []byte:
		return len(v) == 0
	}
	return false
}

// expandToolResultRows applies ctx.Wrap to each logical row (local
// helper; rule 21 — does NOT call spec-1.9 expandToolUseRows since
// spec-1.9 FROZEN; consolidate at spec-1.10 per Q6 ★A).
func expandToolResultRows(ctx Context, rows []toolResultRow) []toolResultRow {
	out := make([]toolResultRow, 0, len(rows))
	for _, r := range rows {
		// Adapter output may contain newlines; split first then wrap each
		// segment via ctx.Wrap policy.
		for _, segment := range strings.Split(r.text, "\n") {
			lines := wrap(segment, ctx.Cols, ctx.Wrap)
			if len(lines) == 0 {
				lines = []string{segment}
			}
			for _, line := range lines {
				out = append(out, toolResultRow{text: line, style: r.style})
			}
		}
	}
	return out
}

// resultErrorPlaceholder returns a 1-row "[render result error: <ToolName>]"
// buffer when an adapter returns an error. block is a leaf renderer;
// errors must not propagate to caller (CLAUDE rule 7). Mirrors
// errorPlaceholder pattern in spec-1.9 toolcall.go (block-leaf contract);
// "[render result error: …]" vs "[render error: …]" suffix distinguishes
// the ToolResult side from ToolUse side. Format echoes CC
// FallbackToolUseErrorMessage (B-14) ancestry — minimal warning row,
// not custom stderr tail (R2 MED-1).
func resultErrorPlaceholder(ctx Context, theme StyleTheme, name string) (buffer.Buffer, error) {
	if ctx.MeasureOnly {
		return measureOnlyBuf(ctx.Cols, 1), nil
	}
	buf, err := buffer.NewGrid(ctx.Cols, 1)
	if err != nil {
		return measureOnlyBuf(ctx.Cols, 1), nil
	}
	msg := fmt.Sprintf("[render result error: %s]", name)
	writeTextRow(buf, 0, 0, msg, theme.Style(StyleWarning), ctx.Cols)
	return buf, nil
}
