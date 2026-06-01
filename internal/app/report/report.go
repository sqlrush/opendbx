// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File report.go — deterministic markdown fault-report generator (spec-1.23
// D-1). Generate is a pure function of (RunSnapshot, now): no time.Now, no map
// iteration for output (tool inputs render via canonical key-sorted JSON), so
// golden/snapshot tests are byte-stable.
//
// The report only RESTRUCTURES what the diagnosis already produced (the user
// question, the tool timeline, and the LLM's final answer). It draws no new
// conclusions of its own (规则 17: the LLM remains the sole diagnostician); it
// adds no LLM call (the answer is already in the snapshot).

package report

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Generate renders a RunSnapshot into a fixed-section markdown report.
// now is the report-generation timestamp (injected for deterministic tests);
// it is rendered in UTC.
func Generate(snap RunSnapshot, now time.Time) string {
	// Redact secrets BEFORE rendering — the report is a durable, user-visible
	// artifact (disk + screen). spec-1.23 R-fix (post-impl codex HIGH-1).
	snap = redactSnapshot(snap)
	var b strings.Builder
	writeHeader(&b, snap, now)
	writeQuestion(&b, snap)
	writeProcess(&b, snap)
	writeConclusion(&b, snap)
	return b.String()
}

func writeHeader(b *strings.Builder, snap RunSnapshot, now time.Time) {
	b.WriteString("# 诊断报告\n\n")
	fmt.Fprintf(b, "- 生成时间: %s\n", now.UTC().Format("2006-01-02T15:04:05Z"))
	fmt.Fprintf(b, "- 轮数: %d\n", snap.Turns)
	fmt.Fprintf(b, "- 状态: %s\n", finishLabel(snap.FinishStatus, snap.TermCode))
	if !snap.StartedAt.IsZero() && !snap.FinishedAt.IsZero() && !snap.FinishedAt.Before(snap.StartedAt) {
		fmt.Fprintf(b, "- 耗时: %s\n", snap.FinishedAt.Sub(snap.StartedAt).Round(time.Millisecond))
	}
}

func writeQuestion(b *strings.Builder, snap RunSnapshot) {
	b.WriteString("\n## 问题\n\n")
	b.WriteString(orPlaceholder(snap.Prompt) + "\n")
}

func writeProcess(b *strings.Builder, snap RunSnapshot) {
	b.WriteString("\n## 诊断过程\n\n")
	if len(snap.ToolTimeline) == 0 {
		b.WriteString("（无工具调用）\n")
		return
	}
	for i := range snap.ToolTimeline {
		te := snap.ToolTimeline[i]
		fmt.Fprintf(b, "### %d. %s%s%s\n\n", i+1, te.Name, cachedTag(te.Cached), errorTag(te.IsError))
		if len(te.Input) > 0 {
			j := canonicalJSON(te.Input)
			f := fence(j)
			fmt.Fprintf(b, "参数:\n\n%sjson\n%s\n%s\n\n", f, j, f)
		}
		res := summarize(te.Result)
		f := fence(res)
		fmt.Fprintf(b, "结果:\n\n%s\n%s\n%s\n\n", f, res, f)
	}
}

// fence returns a backtick run long enough to safely wrap s in a fenced code
// block: one longer than the longest backtick run inside s (min 3). Prevents
// a tool result/input containing ``` from breaking out of the fence and
// spoofing later report sections (spec-1.23 R-fix; post-impl codex LOW-1).
func fence(s string) string {
	longest, cur := 0, 0
	for _, r := range s {
		if r == '`' {
			cur++
			if cur > longest {
				longest = cur
			}
		} else {
			cur = 0
		}
	}
	n := longest + 1
	if n < 3 {
		n = 3
	}
	return strings.Repeat("`", n)
}

func writeConclusion(b *strings.Builder, snap RunSnapshot) {
	b.WriteString("## 结论与建议\n\n")
	if strings.TrimSpace(snap.FinalAnswer) == "" {
		// No final user-visible answer (e.g. abnormal termination, or a run
		// that ended on tool_use). Fall back to a status-derived note so the
		// section is never empty.
		b.WriteString(noAnswerNote(snap.FinishStatus, snap.TermCode) + "\n")
		return
	}
	b.WriteString(snap.FinalAnswer + "\n")
}

func orPlaceholder(s string) string {
	if strings.TrimSpace(s) == "" {
		return "（无）"
	}
	return s
}

func cachedTag(cached bool) string {
	if cached {
		return " (cached)"
	}
	return ""
}

func errorTag(isErr bool) string {
	if isErr {
		return " (error)"
	}
	return ""
}

// canonicalJSON marshals an input map with deterministic key order (Go's
// encoding/json sorts map keys at every nesting level), so report output is
// byte-stable regardless of the map's construction order.
func canonicalJSON(input map[string]any) string {
	raw, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		// Inputs come from llm.DecodeToolInput (JSON-decoded), so Marshal
		// cannot fail; fall open to a stable placeholder rather than panic.
		return "{}"
	}
	return string(raw)
}
