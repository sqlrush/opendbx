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
// Compact format keyed on Content shape. Verbose ctx returns the raw
// content (caller responsibility to format).
func (Read) RenderResult(content any, ctx Context) (string, error) {
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
// equivalent). CC examples: "File not found" / "Error reading file: …".
// opendbx returns the content directly when it already looks like an
// error message; otherwise prefixes "Error: ".
func (Read) RenderErrorResult(content any, _ Context) (string, error) {
	text := contentToString(content)
	if text == "" {
		return "Error reading file", nil
	}
	lower := strings.ToLower(text)
	if strings.HasPrefix(lower, "error") || strings.Contains(lower, "not found") {
		return text, nil
	}
	return "Error: " + text, nil
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
