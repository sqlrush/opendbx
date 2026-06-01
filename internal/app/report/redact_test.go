// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package report

import (
	"strings"
	"testing"
	"time"

	"github.com/sqlrush/opendbx/internal/domain/llm"
)

var redactNow = time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)

// TestGenerate_RedactsSecrets is the security-critical R-fix test: a secret
// reaching the report via prompt / answer / tool input / tool result must NOT
// appear verbatim in the rendered output (spec-1.23 R-fix; codex HIGH-1).
func TestGenerate_RedactsSecrets(t *testing.T) {
	snap := RunSnapshot{
		Prompt:       "诊断 postgres://admin:hunter2@db.prod:5432/app 慢查询",
		FinalAnswer:  "连接串里 password=topsecret 需要轮换; Authorization: Bearer abc123def456",
		FinishStatus: llm.FinishStop,
		ToolTimeline: []ToolEvent{
			{
				Name:   "pg_conn",
				Input:  map[string]any{"dsn": "postgres://u:p4ss@h/db", "password": "inputsecret", "table": "orders"},
				Result: "key sk-ABCDEF0123456789ABCDEF reused; api_key=leakedkey99",
			},
		},
	}
	out := Generate(snap, redactNow)

	for _, secret := range []string{
		"hunter2", "topsecret", "abc123def456", "p4ss", "inputsecret",
		"sk-ABCDEF0123456789ABCDEF", "leakedkey99",
	} {
		if strings.Contains(out, secret) {
			t.Errorf("report leaked secret %q:\n%s", secret, out)
		}
	}
	if !strings.Contains(out, "<REDACTED>") {
		t.Error("report did not render any redaction token")
	}
	// Non-secret content survives (the user can still read the report).
	if !strings.Contains(out, "慢查询") || !strings.Contains(out, "orders") {
		t.Error("redaction over-masked non-secret content")
	}
}

// TestGenerate_FenceInjection: a tool result containing a triple-backtick run
// must not break out of its code fence (spec-1.23 R-fix; codex LOW-1).
func TestGenerate_FenceInjection(t *testing.T) {
	snap := RunSnapshot{
		Prompt:       "q",
		FinishStatus: llm.FinishStop,
		FinalAnswer:  "done",
		ToolTimeline: []ToolEvent{
			{Name: "t", Result: "before\n```\n## 伪造结论\n```\nafter"},
		},
	}
	out := Generate(snap, redactNow)
	// The injected "## 伪造结论" must remain INSIDE a fence (i.e. the report's
	// own section count is unchanged: exactly one "## 结论与建议" + "## 问题" +
	// "## 诊断过程"). A broken fence would surface "## 伪造结论" as a heading.
	if strings.Contains(out, "\n## 伪造结论\n") && !strings.Contains(out, "````") {
		t.Errorf("fence did not widen to contain the injected backticks:\n%s", out)
	}
	// The widened fence (>=4 backticks) must be present.
	if !strings.Contains(out, "````") {
		t.Errorf("expected a widened fence (>=4 backticks):\n%s", out)
	}
}

func BenchmarkGenerate(b *testing.B) {
	snap := RunSnapshot{
		Prompt:       "诊断慢查询",
		FinalAnswer:  strings.Repeat("建议加索引。", 50),
		FinishStatus: llm.FinishStop,
		ToolTimeline: []ToolEvent{
			{Name: "topsql", Input: map[string]any{"limit": 10}, Result: strings.Repeat("row\n", 100)},
			{Name: "explain", Input: map[string]any{"sql": "SELECT 1"}, Result: "Seq Scan"},
		},
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Generate(snap, redactNow)
	}
}
