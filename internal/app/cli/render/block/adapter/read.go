// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File read.go — Read (FileReadTool) tool adapter (spec-1.9 D-4).
// Mirrors CC src/tools/FileReadTool/UI.tsx (§ 1.4 B-8):
//   - default: "path/to/file.ts"
//   - verbose w/ offset+limit: "path/to/file.ts · lines N-M"
//   - PDF w/ pages: "path/to/file.pdf · pages N-M"
//
// Read implements HeaderRenderer only — no Progress / Queued, so
// ToolUse.Render for Running state renders header-only (per spec-1.9
// R2.1.3 HIGH-2 contract).

package adapter

import (
	"fmt"
	"strings"
)

// Read implements HeaderRenderer for the Read / FileReadTool.
type Read struct{}

// RenderHeader returns the file path with optional range suffix per
// CC FileReadTool/UI.tsx:30-65 conventions.
func (Read) RenderHeader(input map[string]any, ctx Context) (string, error) {
	path, ok := input["path"].(string)
	if !ok || path == "" {
		return "(no path)", nil
	}
	// PDF: pages range first (overrides offset/limit).
	if pages, ok := input["pages"].(string); ok && pages != "" {
		return fmt.Sprintf("%s · pages %s", path, pages), nil
	}
	if ctx.Verbose {
		// Verbose: include lines N-M if offset/limit present.
		offset, oOK := readIntField(input, "offset")
		limit, lOK := readIntField(input, "limit")
		switch {
		case oOK && lOK:
			return fmt.Sprintf("%s · lines %d-%d", path, offset, offset+limit-1), nil
		case oOK:
			return fmt.Sprintf("%s · from line %d", path, offset), nil
		case lOK:
			return fmt.Sprintf("%s · first %d lines", path, limit), nil
		}
	}
	return path, nil
}

func readIntField(m map[string]any, key string) (int, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	case string:
		// LLM input may be string; parse defensively.
		var parsed int
		_, err := fmt.Sscanf(strings.TrimSpace(n), "%d", &parsed)
		return parsed, err == nil
	}
	return 0, false
}

func init() {
	Default.Register("Read", Read{})
}
