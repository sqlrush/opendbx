// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File tool_echo.go — minimal "echo" ToolExecutor (spec-1.21 D-3 built-
// in). Pure input echo: JSON-renders whatever the model passed and
// returns it back as Content. NO shell, NO SQL, NO filesystem — the
// scope is deliberately narrowed at the T-5 boundary so a misuse cannot
// leak a real execution surface (user T-5 instruction).
//
// Intended use: end-to-end loop validation (LLM sends args, sees them
// in the next turn's tool_result). Real read tools land in spec-2.1.

package diagnose

import (
	"context"
	"encoding/json"

	"github.com/sqlrush/opendbx/internal/domain/llm"
)

// EchoTool round-trips its input map verbatim as a JSON-encoded string.
// Implementations never touch IO, the filesystem, or any external
// system. IsError is always false — a malformed argument shape merely
// gets echoed back so the LLM can correct itself on the next turn.
type EchoTool struct{}

// Name implements ToolExecutor.
func (EchoTool) Name() string { return "echo" }

// Schema implements ToolExecutor. A free-form object input — the model
// may pass any JSON-encodable map; the tool returns it verbatim.
func (EchoTool) Schema() llm.ToolSchema {
	return llm.ToolSchema{
		Name:        "echo",
		Description: "Return the JSON-encoded input verbatim. Pure echo; no side effects.",
		InputSchema: map[string]any{
			"type": "object",
			// No "required" list — echo accepts any (or empty) input.
			"properties":           map[string]any{},
			"additionalProperties": true,
		},
	}
}

// Execute implements ToolExecutor. nil input → "{}".
func (EchoTool) Execute(ctx context.Context, input map[string]any) (ToolOutput, error) {
	// errcode-lint:exempt -- spec-1.21 D-3 / D-4: ctx errors pass through unchanged; Loop classifies cancel-vs-timeout (D-4) via errors.Is on context.{Canceled,DeadlineExceeded}.
	if err := ctx.Err(); err != nil {
		return ToolOutput{}, err
	}
	if input == nil {
		return ToolOutput{Content: "{}"}, nil
	}
	b, err := json.Marshal(input)
	if err != nil {
		// Should not happen for map[string]any of JSON-decodable values
		// (DecodeToolInput guards upstream), but if it does we report
		// it as a recoverable IsError so the model sees the failure
		// without us swallowing it.
		return ToolOutput{Content: "echo: cannot marshal input: " + err.Error(), IsError: true}, nil
	}
	return ToolOutput{Content: string(b)}, nil
}
