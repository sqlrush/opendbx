// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File redact.go — secret redaction for the fault report (spec-1.23 R-fix;
// three-route post-impl review codex HIGH-1 / code-reviewer MED-2).
//
// The report is a DURABLE, user-visible artifact (written to <config-dir>/
// reports/*.md AND rendered to screen). A DSN / password / api_key / Bearer
// token can reach it three ways: pasted in the prompt, embedded in a tool
// input/result, or echoed in the LLM's final answer. We mask all of them with
// the same battle-tested redactor the logger uses (logger.RedactString /
// RedactValue) BEFORE rendering — so neither the disk file nor the screen
// shows a verbatim secret.

package report

import "github.com/sqlrush/opendbx/internal/platform/logger"

// redactSnapshot returns a copy of snap with secret-bearing values masked. The
// input is not mutated (规则 12 immutability): the prompt/answer strings are
// pattern-masked, tool results are pattern-masked, and tool input maps are
// deep-redacted (key-name + value patterns).
func redactSnapshot(snap RunSnapshot) RunSnapshot {
	out := snap
	out.Prompt = logger.RedactString(snap.Prompt)
	out.FinalAnswer = logger.RedactString(snap.FinalAnswer)
	if len(snap.ToolTimeline) > 0 {
		tl := make([]ToolEvent, len(snap.ToolTimeline))
		for i, te := range snap.ToolTimeline {
			te.Result = logger.RedactString(te.Result)
			if te.Input != nil {
				if m, ok := logger.RedactValue(te.Input).(map[string]any); ok {
					te.Input = m
				}
			}
			tl[i] = te
		}
		out.ToolTimeline = tl
	}
	return out
}
