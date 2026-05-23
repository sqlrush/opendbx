// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File bash_result.go — Bash ToolResult-side adapter methods (spec-1.9b
// D-3). Per CC BashTool/UI.tsx:159-184 (B-14): renderToolResultMessage
// delegates to BashToolResultMessage component (success); renderToolUse
// ErrorMessage delegates to FallbackToolUseErrorMessage (R2 MED-1: NOT
// custom tail-of-stderr). opendbx ports these as compact text per row.

package adapter

import (
	"fmt"
	"strings"
)

// RenderResult formats Bash success output (BashToolResultMessage-equivalent
// per B-14 :159-173). Truncates to a compact 2-row preview; verbose ctx
// expands rows. Content can be string, []byte, or arbitrary value
// (fmt.Sprintf "%v" fallback).
func (Bash) RenderResult(content any, ctx Context) (string, error) {
	text := contentToString(content)
	if text == "" {
		return "(empty)", nil
	}
	if ctx.Verbose {
		return text, nil
	}
	// Compact: ≤ 2 lines + tail truncate (mirror CC short-form preview).
	lines := strings.SplitN(text, "\n", 3)
	if len(lines) > 2 {
		lines = lines[:2]
		lines[1] = ellipsizeLine(lines[1])
	}
	return strings.Join(lines, "\n"), nil
}

// RenderRejected formats Bash rejected output. Per spec-1.9b Q8 ★A + R3
// HIGH-2 the canonical Canceled / fallback-Rejected text is the CC
// InterruptedByUser string, owned by block.ToolResult.Render. Bash's
// RenderRejected provides a verbose context dump (the rejection reason
// content) but defers to block-level fallback when content is empty.
func (Bash) RenderRejected(content any, _ Context) (string, error) {
	text := contentToString(content)
	if text == "" {
		// Empty content → return empty so caller uses CC fixed fallback.
		return "", nil
	}
	return text, nil
}

// RenderErrorResult formats Bash error output (FallbackToolUseErrorMessage-
// equivalent per B-14 :174-184; R2 MED-1: NOT custom stderr tail). Generic
// content display compact 2-row.
func (Bash) RenderErrorResult(content any, ctx Context) (string, error) {
	text := contentToString(content)
	if text == "" {
		return "(no error message)", nil
	}
	if ctx.Verbose {
		return text, nil
	}
	lines := strings.SplitN(text, "\n", 3)
	if len(lines) > 2 {
		lines = lines[:2]
		lines[1] = ellipsizeLine(lines[1])
	}
	return strings.Join(lines, "\n"), nil
}

// contentToString safely converts `any` content to string for adapter
// rendering. string / []byte natively; everything else via fmt.Sprintf.
func contentToString(content any) string {
	switch v := content.(type) {
	case nil:
		return ""
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return fmt.Sprintf("%v", v)
	}
}
