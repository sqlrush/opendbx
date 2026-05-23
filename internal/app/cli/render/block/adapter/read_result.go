// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File read_result.go — Read ToolResult-side adapter methods (spec-1.9b
// D-3 / R2 HIGH-2 / B-15). Per CC FileReadTool/UI.tsx (B-15): Read
// implements BOTH renderToolResultMessage (:77-142) + renderToolUseError
// Message (:144-164). R1 误说 "Read 仅 ResultRenderer"; R2 HIGH-2 absorb
// 加 ErrorResultRenderer.
//
// Read does NOT implement RejectedRenderer (Q3 ★A; CC Read also doesn't
// in observed scope).

package adapter

import (
	"fmt"
	"strings"
)

// RenderResult formats Read success output (FileReadTool/UI.tsx:77-142
// equivalent). Examples:
//
//	"Read 42 lines"             (text file with line count)
//	"Read N lines (truncated)"  (truncated read)
//	"image"                     (binary image)
//	"PDF · 3 pages"             (PDF preview)
//
// Compact format keyed on Content shape. Structured CC output maps always
// render as summaries; verbose ctx returns raw string content for legacy
// string/[]byte paths.
func (Read) RenderResult(content any, ctx Context) (string, error) {
	if m, ok := mapFromContent(content); ok {
		if summary, ok := readSummaryFromMap(m); ok {
			return summary, nil
		}
	}
	if ctx.Verbose {
		return contentToString(content), nil
	}
	switch v := content.(type) {
	case nil:
		return "(empty file)", nil
	case string:
		return readSummaryFromString(v), nil
	case []byte:
		return readSummaryFromString(string(v)), nil
	default:
		return fmt.Sprintf("%v", v), nil
	}
}

// RenderErrorResult formats Read error output (FileReadTool/UI.tsx:144-164
// equivalent). Read has two compact special cases, then delegates to the
// shared CC fallback formatter.
func (Read) RenderErrorResult(content any, ctx Context) (string, error) {
	if !ctx.Verbose {
		if text, ok := content.(string); ok {
			if strings.Contains(text, fileNotFoundCWDNote) {
				return "File not found", nil
			}
			if _, found := extractTag(text, "tool_use_error"); found {
				return "Error reading file", nil
			}
		}
	}
	return FormatFallbackToolUseError(content, ctx), nil
}

// readSummaryFromString produces a "Read N lines" preview without
// dumping file content (matches CC :77-142 compact mode).
func readSummaryFromString(s string) string {
	if s == "" {
		return "(empty file)"
	}
	n := strings.Count(s, "\n")
	// Trailing newline shouldn't inflate line count.
	if strings.HasSuffix(s, "\n") {
		// Lines = count of \n if file ends with newline.
	} else {
		n++ // last line without trailing \n
	}
	if n <= 0 {
		n = 1
	}
	return fmt.Sprintf("Read %d lines", n)
}

const fileNotFoundCWDNote = "Note: your current working directory is"

func readSummaryFromMap(m map[string]any) (string, bool) {
	typ := stringFromMap(m, "type")
	file, _ := mapFromContent(m["file"])
	switch typ {
	case "image":
		size, _ := intFromMap(file, "originalSize")
		return fmt.Sprintf("Read image (%s)", formatFileSize(size)), true
	case "notebook":
		count := sliceLen(file["cells"])
		if count < 1 {
			return "No cells found in notebook", true
		}
		return fmt.Sprintf("Read %d cells", count), true
	case "pdf":
		size, _ := intFromMap(file, "originalSize")
		return fmt.Sprintf("Read PDF (%s)", formatFileSize(size)), true
	case "parts":
		count, _ := intFromMap(file, "count")
		size, _ := intFromMap(file, "originalSize")
		return fmt.Sprintf("Read %d %s (%s)", count, plural(count, "page", "pages"), formatFileSize(size)), true
	case "text":
		lines, _ := intFromMap(file, "numLines")
		return fmt.Sprintf("Read %d %s", lines, plural(lines, "line", "lines")), true
	case "file_unchanged":
		return "Unchanged since last read", true
	default:
		return "", false
	}
}

func sliceLen(v any) int {
	switch s := v.(type) {
	case []any:
		return len(s)
	case []map[string]any:
		return len(s)
	case []string:
		return len(s)
	default:
		return 0
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func formatFileSize(size int) string {
	kb := float64(size) / 1024
	if kb < 1 {
		return fmt.Sprintf("%d bytes", size)
	}
	if kb < 1024 {
		return trimTrailingZero(kb) + "KB"
	}
	mb := kb / 1024
	if mb < 1024 {
		return trimTrailingZero(mb) + "MB"
	}
	return trimTrailingZero(mb/1024) + "GB"
}

func trimTrailingZero(n float64) string {
	s := fmt.Sprintf("%.1f", n)
	return strings.TrimSuffix(s, ".0")
}
