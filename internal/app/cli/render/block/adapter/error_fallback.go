// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File error_fallback.go — CC FallbackToolUseErrorMessage-equivalent
// formatter for ToolResult-side error rows (spec-1.9b R4).

package adapter

import "strings"

const maxFallbackErrorLines = 10

// FormatFallbackToolUseError mirrors CC FallbackToolUseErrorMessage:
// non-string content uses a fixed generic failure, string content strips
// model-only tags, normalizes the leading "Error: " prefix, and compact
// mode keeps the first 10 lines.
func FormatFallbackToolUseError(content any, ctx Context) string {
	var text string
	s, ok := content.(string)
	if !ok {
		text = "Tool execution failed"
	} else {
		if extracted, found := extractTag(s, "tool_use_error"); found {
			s = extracted
		}
		s = removeTagBlock(s, "sandbox_violations")
		s = stripSimpleTags(s, "error")
		trimmed := strings.TrimSpace(s)

		switch {
		case !ctx.Verbose && strings.Contains(trimmed, "InputValidationError: "):
			text = "Invalid tool parameters"
		case strings.HasPrefix(trimmed, "Error: ") || strings.HasPrefix(trimmed, "Cancelled: "):
			text = trimmed
		default:
			text = "Error: " + trimmed
		}
	}
	if !ctx.Verbose {
		text = limitLines(text, maxFallbackErrorLines)
	}
	return text
}

func extractTag(text, tag string) (string, bool) {
	open := "<" + tag + ">"
	close := "</" + tag + ">"
	start := strings.Index(text, open)
	if start < 0 {
		return "", false
	}
	start += len(open)
	end := strings.Index(text[start:], close)
	if end < 0 {
		return "", false
	}
	return text[start : start+end], true
}

func removeTagBlock(text, tag string) string {
	open := "<" + tag + ">"
	close := "</" + tag + ">"
	for {
		start := strings.Index(text, open)
		if start < 0 {
			return text
		}
		endRel := strings.Index(text[start+len(open):], close)
		if endRel < 0 {
			return text
		}
		end := start + len(open) + endRel + len(close)
		text = text[:start] + text[end:]
	}
}

func stripSimpleTags(text, tag string) string {
	text = strings.ReplaceAll(text, "<"+tag+">", "")
	return strings.ReplaceAll(text, "</"+tag+">", "")
}

func limitLines(text string, maxLines int) string {
	if maxLines <= 0 {
		return ""
	}
	lines := strings.Split(text, "\n")
	if len(lines) <= maxLines {
		return text
	}
	return strings.Join(lines[:maxLines], "\n")
}
