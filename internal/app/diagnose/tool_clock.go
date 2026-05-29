// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File tool_clock.go — minimal "clock" ToolExecutor (spec-1.21 D-3 built-
// in). No side effects, no input. Returns the current time as an RFC3339
// string. Time source is injectable for tests (default time.Now).
//
// Scope discipline (T-5 boundary): this is a pure-read, no-permission
// tool intended to prove the loop end-to-end. Real DB-touching skills
// (`topsql`, `awr`, `pg_settings_*`) land in spec-2.1; do NOT extend
// this file with shell, SQL, or filesystem access.

package diagnose

import (
	"context"
	"time"

	"github.com/sqlrush/opendbx/internal/domain/llm"
)

// ClockTool returns the current time. Configurable Now() lets tests
// inject a fixed timestamp without monkey-patching time.Now.
type ClockTool struct {
	// Now returns the current time. nil → time.Now.
	Now func() time.Time
}

// Name implements ToolExecutor.
func (ClockTool) Name() string { return "clock" }

// Schema implements ToolExecutor. No input parameters; the model invokes
// it bare. We still ship a valid JSON-schema object so OpenAI-compat
// providers that require Parameters do not 400.
func (c ClockTool) Schema() llm.ToolSchema {
	return llm.ToolSchema{
		Name:        "clock",
		Description: "Return the current time (RFC3339, UTC). No input parameters.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}
}

// Execute implements ToolExecutor. ctx is honored before producing
// output so a cancelled / deadline-expired loop does not waste a tick.
func (c ClockTool) Execute(ctx context.Context, _ map[string]any) (ToolOutput, error) {
	// errcode-lint:exempt -- spec-1.21 D-3 / D-4: ctx errors pass through unchanged; Loop classifies cancel-vs-timeout (D-4) via errors.Is on context.{Canceled,DeadlineExceeded}.
	if err := ctx.Err(); err != nil {
		return ToolOutput{}, err
	}
	now := c.Now
	if now == nil {
		now = time.Now
	}
	return ToolOutput{Content: now().UTC().Format(time.RFC3339)}, nil
}
