// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File generic_result.go — Generic fallback ToolResult-side adapter
// methods (spec-1.9b D-3 / Q3 ★A). Generic implements RenderResult
// minimally; intentionally does NOT implement RejectedRenderer or
// ErrorResultRenderer (block-level fallback covers those paths).

package adapter

// RenderResult is the minimal Generic fallback for unknown-tool
// ToolResult success path. Returns "(result)" or formatted Content
// when present.
func (Generic) RenderResult(content any, _ Context) (string, error) {
	text := contentToString(content)
	if text == "" {
		return "(result)", nil
	}
	return text, nil
}
