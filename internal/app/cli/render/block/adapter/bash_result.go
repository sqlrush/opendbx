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
	"math"
	"strings"
)

// RenderResult formats Bash success output (BashToolResultMessage-equivalent
// per B-14 :159-173). Truncates to a compact 2-row preview; verbose ctx
// expands rows. Content can be the CC Bash Out object, string, []byte, or
// arbitrary value (fmt.Sprintf "%v" fallback).
func (Bash) RenderResult(content any, ctx Context) (string, error) {
	text, ok := bashOutputText(content)
	if !ok {
		text = contentToString(content)
	}
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

// RenderErrorResult formats Bash error output via the shared CC
// FallbackToolUseErrorMessage-equivalent. R2 MED-1: this is NOT a custom
// stderr tail renderer.
func (Bash) RenderErrorResult(content any, ctx Context) (string, error) {
	return FormatFallbackToolUseError(content, ctx), nil
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

func bashOutputText(content any) (string, bool) {
	m, ok := mapFromContent(content)
	if !ok {
		return "", false
	}
	if boolFromMap(m, "isImage") {
		return "[Image data detected and sent to Claude]", true
	}

	stdout := stringFromMap(m, "stdout")
	stderr := strings.TrimSpace(removeTagBlock(stringFromMap(m, "stderr"), "sandbox_violations"))
	parts := make([]string, 0, 2)
	if stdout != "" {
		parts = append(parts, stdout)
	}
	if stderr != "" {
		parts = append(parts, stderr)
	}
	if len(parts) > 0 {
		return strings.Join(parts, "\n"), true
	}
	if stringFromMap(m, "backgroundTaskId") != "" {
		return "Running in the background", true
	}
	if interpretation := stringFromMap(m, "returnCodeInterpretation"); interpretation != "" {
		return interpretation, true
	}
	if boolFromMap(m, "noOutputExpected") {
		return "Done", true
	}
	return "(No output)", true
}

func mapFromContent(content any) (map[string]any, bool) {
	switch v := content.(type) {
	case map[string]any:
		return v, true
	case map[string]string:
		out := make(map[string]any, len(v))
		for k, val := range v {
			out[k] = val
		}
		return out, true
	default:
		return nil, false
	}
}

func stringFromMap(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	default:
		return ""
	}
}

func boolFromMap(m map[string]any, key string) bool {
	v, ok := m[key]
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}

func intFromMap(m map[string]any, key string) (int, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case int:
		return n, true
	case int8:
		return int(n), true
	case int16:
		return int(n), true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case uint:
		// Clamp to math.MaxInt to satisfy gosec G115; file size /
		// line counts beyond MaxInt are not realistic for tool output.
		if n > uint(math.MaxInt) {
			return math.MaxInt, true
		}
		return int(n), true
	case uint8:
		return int(n), true
	case uint16:
		return int(n), true
	case uint32:
		return int(n), true
	case uint64:
		if n > uint64(math.MaxInt) {
			return math.MaxInt, true
		}
		return int(n), true
	case float32:
		return int(n), true
	case float64:
		return int(n), true
	case string:
		var parsed int
		_, err := fmt.Sscanf(strings.TrimSpace(n), "%d", &parsed)
		return parsed, err == nil
	default:
		return 0, false
	}
}
