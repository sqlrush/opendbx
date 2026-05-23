// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File bash.go — Bash tool adapter (spec-1.9 D-3).
// Mirrors CC src/tools/BashTool/UI.tsx (§ 1.4 B-6/B-7):
//   - non-verbose: command truncated to 160 char + ≤2 line (BashTool/UI.tsx:85-130)
//   - verbose: full command (no truncate)
//   - Queued: "Waiting…" (BashTool/UI.tsx:154-158)
//   - Running (no progress): "Running…" (BashTool/UI.tsx:148)
//   - Running (with progress): ShellProgressMessage (BashTool/UI.tsx:131-153)

package adapter

import (
	"fmt"
	"strings"
)

const (
	// Per CC BashTool/UI.tsx MAX_COMMAND_DISPLAY_CHARS / _LINES.
	bashMaxDisplayChars = 160
	bashMaxDisplayLines = 2
)

// Bash implements HeaderRenderer + ProgressRenderer + QueuedRenderer
// (the full ToolUse subset of CC Tool.ts:605-667).
type Bash struct{}

// RenderHeader returns the Bash command preview per CC behavior.
//   - Empty Input → "(no command)" placeholder
//   - input["command"] non-string → "(invalid command)"
//   - verbose=false: truncate to 160 char + ≤2 lines (CC convention)
//   - verbose=true: full command
func (Bash) RenderHeader(input map[string]any, ctx Context) (string, error) {
	cmd, ok := input["command"].(string)
	if !ok || cmd == "" {
		return "(no command)", nil
	}
	if ctx.Verbose {
		return cmd, nil
	}
	return truncateCommand(cmd), nil
}

// RenderProgress returns the second-row progress text. When progress
// is nil/empty, returns "Running…" per CC BashTool/UI.tsx:148.
// Otherwise summarizes the latest ProgressMessage.
func (Bash) RenderProgress(progress []ProgressMessage, _ Context) (string, error) {
	if len(progress) == 0 {
		return "Running…", nil
	}
	last := progress[len(progress)-1]
	parts := make([]string, 0, 4)
	if last.ElapsedSeconds > 0 {
		parts = append(parts, fmt.Sprintf("%ds", last.ElapsedSeconds))
	}
	if last.TotalLines > 0 {
		parts = append(parts, fmt.Sprintf("%d lines", last.TotalLines))
	}
	if last.TotalBytes > 0 {
		parts = append(parts, fmt.Sprintf("%dB", last.TotalBytes))
	}
	if last.TimeoutMs > 0 {
		parts = append(parts, fmt.Sprintf("timeout %dms", last.TimeoutMs))
	}
	if len(parts) == 0 {
		return "Running…", nil
	}
	return "Running… (" + strings.Join(parts, ", ") + ")", nil
}

// RenderQueued returns "Waiting…" per CC BashTool/UI.tsx:154-158.
func (Bash) RenderQueued() (string, error) {
	return "Waiting…", nil
}

// truncateCommand applies CC's MAX_COMMAND_DISPLAY_CHARS / _LINES
// limits. Long single line → first 160 chars + "…". Multi-line >2 →
// first 2 lines + "…" on second.
func truncateCommand(cmd string) string {
	// Normalize: keep at most 2 lines.
	lines := strings.SplitN(cmd, "\n", 3)
	if len(lines) > bashMaxDisplayLines {
		lines = lines[:bashMaxDisplayLines]
		// Indicate truncation on the last kept line.
		lines[len(lines)-1] = ellipsizeLine(lines[len(lines)-1])
	}
	out := strings.Join(lines, "\n")
	if len(out) > bashMaxDisplayChars {
		out = out[:bashMaxDisplayChars-1] + "…"
	}
	return out
}

func ellipsizeLine(s string) string {
	if len(s) > 0 && !strings.HasSuffix(s, "…") {
		return s + "…"
	}
	return s
}

func init() {
	Default.Register("Bash", Bash{})
}
