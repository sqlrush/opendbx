// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File tool.go — ToolExecutor interface + ToolOutput (spec-1.21 D-3).
//
// ToolExecutor is the orchestration-side contract used by Loop:
//   - Name() identifies the tool; MUST equal Schema().Name (Registry
//     enforces this invariant at registration time).
//   - Schema() supplies the JSON-schema-shaped descriptor the LLM sees
//     via Request.Tools (spec-1.20 D-4 decode-only schema).
//   - Execute(ctx, input) runs the tool. ctx carries a per-tool deadline
//     (default 30s, DiagnoseConfig.ToolTimeout); the executor MUST
//     respect ctx — long-running work has to honor ctx.Done(). Real
//     DB-touching skills land in spec-2.1.
//
// Two failure shapes are kept distinct (R2 signature 统一, user 拍板):
//   - ToolOutput{IsError: true} — recoverable / semantic failure that
//     the LLM should self-correct on (validation error, tool-side
//     refusal, partial result). Loop writes Content into the
//     BlockToolResult and the conversation continues.
//   - Go `error` return — fatal / infrastructure failure (ctx cancel,
//     deadline exceeded, panic, SDK transport blow-up). Loop handles
//     this out-of-band per spec-1.21 D-4 cancel-vs-timeout three-way
//     dispatch and may terminate the run.

package diagnose

import (
	"context"

	"github.com/sqlrush/opendbx/internal/domain/llm"
)

// ToolOutput is the recoverable result envelope. Content maps 1:1 to the
// text payload of the resulting BlockToolResult; IsError signals that the
// model should self-correct on the next turn (NOT a hard termination).
type ToolOutput struct {
	Content string
	IsError bool
	// ToolFilter, when non-nil, REPLACES the execution-tool scope for the
	// remainder of the current Run (spec-2.3 allowed-tools enforcement).
	// nil = no scope change (inherit current scope). "Skill" is implicitly
	// retained by the Loop as the scope-control verb (spec-2.3 Q13) — it
	// never needs to be listed. The field is orchestration-control only:
	// classifyToolErr copies Content/IsError exclusively, so ToolFilter is
	// NEVER serialized to the provider wire or the transcript.
	ToolFilter []string
}

// ToolExecutor is implemented by every diagnose-loop-callable tool.
//
// Concurrency: Execute MAY be called serially from a single Loop
// goroutine (spec-1.21 D-4); implementations need not be safe for
// concurrent use across goroutines unless explicitly documented.
type ToolExecutor interface {
	Name() string
	Schema() llm.ToolSchema
	Execute(ctx context.Context, input map[string]any) (ToolOutput, error)
}
