// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File toolcall.go — production ToolUse block (spec-1.9 D-1).
// spec-1.9 R2.1.3 bifurcation: ToolUse only (ToolResult → spec-1.9b).
// 5-enum state (Queued/Running/WaitingPermission/Resolved/Error) per CC
// AssistantToolUseMessage.tsx B-4; Progress is **not** a state — it is a
// Running-branch render hook driven by ProgressMessages != nil + adapter
// implementing ProgressRenderer (spec-1.9 R2.1.3 HIGH-1).

package block

import (
	"fmt"

	"github.com/sqlrush/opendbx/internal/app/cli/render/block/adapter"
	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
)

// ToolUseState enumerates the 5 lifecycle states of an assistant tool
// invocation, mirroring CC AssistantToolUseMessage.tsx state derivation
// (spec-1.9 § 1.4 B-4):
//
//   - StateQueued:             param.id 不在 inProgressToolUseIDs 且未 resolved
//     (CC AssistantToolUseMessage.tsx:113)
//   - StateRunning:            param.id 在 inProgressToolUseIDs 且未 resolved
//     (id.:113 isQueued complement). **Progress 不是独立 state**, 而是
//     Running 下 ProgressMessages + adapter ProgressRenderer 触发的渲染分支
//     (CC `!isResolved && !isQueued` → renderToolUseProgressMessage;
//     id.:240-244)
//   - StateWaitingPermission:  pendingWorkerRequest?.toolUseId === param.id
//     (id.:122)
//   - StateResolved:           lookups.resolvedToolUseIDs.has(param.id)
//     (id.:103)
//   - StateError:              lookups.erroredToolUseIDs.has(param.id)
//     (id.:186)
type ToolUseState int

// Lifecycle constants for ToolUseState (spec-1.9 D-1).
const (
	StateQueued ToolUseState = iota
	StateRunning
	StateWaitingPermission
	StateResolved
	StateError
)

// ToolUse is the spec-0.13 D-3 toolcall block type. spec-1.9 promotes it
// from stub (ErrUnsupportedNode) to production via the bifurcated
// design per spec-1.9 R2 HIGH-1 (ToolResult is owned by spec-1.9b).
//
// Maps to Anthropic SDK `ToolUseBlockParam` (CC B-3
// AssistantToolUseMessage.tsx:22-34):
//   - ID:               param.id — Anthropic tool-use UUID; used for state
//     lookup + spec-1.9b ToolResult.ToolUseID back-reference
//   - Name:             param.name — tool name; adapter Registry lookup key
//     (e.g., "Bash", "Read")
//   - Input:            param.input — raw input map; not pre-formatted.
//     adapter RenderHeader interprets per tool (Bash reads .command,
//     Read reads .path / .offset / .limit / .pages)
//   - State:            5-enum; caller (spec-1.21 diagnose-loop) owns
//     transitions, block is stateless and re-renders per State on each tick
//   - ProgressMessages: only meaningful when State == StateRunning.
//     If adapter implements ProgressRenderer, Render always invokes it
//     (nil/empty slice allowed; Bash empty progress renders "Running…").
//     Otherwise header-only (Read/Generic default).
//
// **R1 fields removed by spec-1.9 R2/R2.1**:
//   - Args string:       adapter SSOT, not stored on block (R2 HIGH-4)
//   - Result string:     spec-1.9b ToolResult.Content (R2 HIGH-1)
//   - Collapsed bool:    Ctrl+O is global mode via ctx.Verbose injected by
//     spec-1.15 TUI program (R2 HIGH-3 / CC B-9)
//   - DurationMs int:    spec-3.8 cost-tracker owns timing (❌-8)
type ToolUse struct {
	ID               string
	Name             string
	Input            map[string]any
	State            ToolUseState
	ProgressMessages []adapter.ProgressMessage
}

// NewToolUse constructs a ToolUse with State=StateQueued init default
// (CC AssistantToolUseMessage.tsx:113 derived state 起点).
func NewToolUse(id, name string, input map[string]any) ToolUse {
	return ToolUse{
		ID:    id,
		Name:  name,
		Input: input,
		State: StateQueued,
	}
}

// Render produces a Buffer per spec-1.9 D-6 (R2.1/R2.1.3 layout matrix).
//
// Per-state layout:
//
//	StateQueued:
//	  "<indicator> <queued-text>"
//	  queued-text from adapter.QueuedRenderer if implemented else "Waiting…"
//	StateRunning:
//	  "<indicator> <header>"
//	  + IF adapter implements ProgressRenderer THEN
//	      <progress-text>  (may be multi-line; nil/empty progress OK;
//	       Bash returns "Running…" for empty per BashTool/UI.tsx:148)
//	    ELSE header-only (Read/Generic default)
//	StateWaitingPermission:
//	  "<indicator> <header>"
//	  + dim row "Waiting for permission…"
//	    (AssistantToolUseMessage.tsx:240; R2.1.3 HIGH-3 verified)
//	StateResolved:
//	  "<indicator> <header>"  (single row, no progress)
//	StateError:
//	  "<indicator> <header>"  (StyleWarning indicator + dim error row if Input has "_error")
//
// Adapter dispatch: adapter.Default.Lookup(t.Name); fallback Generic
// (which expects "_name" synthetic key — Render adds it).
//
// MeasureOnly fast path: returns Buffer with correct Size().rows but
// cells no-op (spec-1.7 D-1 contract).
func (t ToolUse) Render(ctx Context) (buffer.Buffer, error) {
	if ctx.Cols <= 0 {
		return measureOnlyBuf(0, 0), nil
	}
	theme := themeOrDefault(ctx.Theme)
	rdr := adapter.Default.Lookup(t.Name)
	useGeneric := rdr == nil
	adapterCtx := adapter.Context{
		Verbose: ctx.Verbose, // spec-1.7 R3 / spec-1.9 D-6 Q4 ★A propagation
		Cols:    ctx.Cols,
	}

	// Collect rows to render (each row = string + style).
	type row struct {
		text  string
		style StyleKind
	}
	var rows []row

	// Resolve header per state.
	headerText, err := resolveHeader(rdr, useGeneric, t.Name, t.Input, adapterCtx)
	if err != nil {
		return errorPlaceholder(ctx, theme, t.Name)
	}
	ind := stateIndicator(t.State)
	headerStyle := StyleNormal
	if t.State == StateWaitingPermission || t.State == StateError {
		headerStyle = ind.Style
	}
	headerLine := fmt.Sprintf("%c %s", ind.Rune, headerText)
	rows = append(rows, row{text: headerLine, style: headerStyle})

	// Add per-state secondary rows.
	switch t.State {
	case StateQueued:
		// Header replaced by Queued text directly; ignore secondary row.
		if qr, ok := rdr.(adapter.QueuedRenderer); ok {
			qt, qerr := qr.RenderQueued()
			if qerr == nil && qt != "" {
				// Replace header row text with queued text (still
				// prefix with indicator).
				rows[0].text = fmt.Sprintf("%c %s", ind.Rune, qt)
			}
		} else {
			rows[0].text = fmt.Sprintf("%c Waiting…", ind.Rune)
		}
	case StateRunning:
		if pr, ok := rdr.(adapter.ProgressRenderer); ok {
			pt, perr := pr.RenderProgress(t.ProgressMessages, adapterCtx)
			if perr == nil && pt != "" {
				rows = append(rows, row{text: pt, style: StyleDimmed})
			}
		}
	case StateWaitingPermission:
		rows = append(rows, row{text: "Waiting for permission…", style: StyleDimmed})
	case StateResolved:
		// header-only
	case StateError:
		if msg, _ := t.Input["_error"].(string); msg != "" {
			rows = append(rows, row{text: msg, style: StyleDimmed})
		}
	}

	if ctx.MeasureOnly {
		return measureOnlyBuf(ctx.Cols, len(rows)), nil
	}
	buf, err := buffer.NewGrid(ctx.Cols, len(rows))
	if err != nil {
		return measureOnlyBuf(ctx.Cols, len(rows)), nil
	}
	for y, r := range rows {
		writeTextRow(buf, 0, y, r.text, theme.Style(r.style), ctx.Cols)
	}
	return buf, nil
}

// resolveHeader runs the appropriate adapter.RenderHeader call. If
// using Generic fallback, injects "_name" synthetic key so Generic
// can show the tool name.
func resolveHeader(rdr adapter.HeaderRenderer, useGeneric bool, name string, input map[string]any, actx adapter.Context) (string, error) {
	if useGeneric {
		// Generic adapter; clone input + inject _name.
		in := make(map[string]any, len(input)+1)
		for k, v := range input {
			in[k] = v
		}
		in["_name"] = name
		return adapter.Generic{}.RenderHeader(in, actx)
	}
	if name == "" {
		return "(unnamed tool)", nil
	}
	return rdr.RenderHeader(input, actx)
}

// errorPlaceholder returns a 1-row "[render error: <name>]" buffer
// when an adapter returns an error from RenderHeader. block is a leaf
// renderer; errors must not propagate to caller (CLAUDE rule 7
// rendering contract).
func errorPlaceholder(ctx Context, theme StyleTheme, name string) (buffer.Buffer, error) {
	if ctx.MeasureOnly {
		return measureOnlyBuf(ctx.Cols, 1), nil
	}
	buf, err := buffer.NewGrid(ctx.Cols, 1)
	if err != nil {
		return measureOnlyBuf(ctx.Cols, 1), nil
	}
	msg := fmt.Sprintf("[render error: %s]", name)
	writeTextRow(buf, 0, 0, msg, theme.Style(StyleWarning), ctx.Cols)
	return buf, nil
}
