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

func fixedNow() time.Time {
	return time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
}

// TestGenerate_Golden pins the exact markdown for a minimal complete run.
func TestGenerate_Golden(t *testing.T) {
	t.Parallel()
	snap := RunSnapshot{
		Prompt:       "为什么慢",
		FinalAnswer:  "因为缺索引",
		Turns:        2,
		FinishStatus: llm.FinishStop,
		ToolTimeline: []ToolEvent{
			{Name: "echo", Input: map[string]any{"x": "1"}, Result: "ok"},
		},
	}
	want := "# 诊断报告\n\n" +
		"- 生成时间: 2026-05-31T12:00:00Z\n" +
		"- 轮数: 2\n" +
		"- 状态: 正常完成\n" +
		"\n## 问题\n\n" +
		"为什么慢\n" +
		"\n## 诊断过程\n\n" +
		"### 1. echo\n\n" +
		"参数:\n\n```json\n{\n  \"x\": \"1\"\n}\n```\n\n" +
		"结果:\n\n```\nok\n```\n\n" +
		"## 结论与建议\n\n" +
		"因为缺索引\n"
	got := Generate(snap, fixedNow())
	if got != want {
		t.Errorf("golden mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestGenerate_Deterministic — same (snapshot, now) → byte-identical across
// runs, and tool Input renders with sorted keys regardless of map order.
func TestGenerate_Deterministic(t *testing.T) {
	t.Parallel()
	snap := RunSnapshot{
		Prompt:       "q",
		FinalAnswer:  "a",
		FinishStatus: llm.FinishStop,
		ToolTimeline: []ToolEvent{
			{Name: "t", Input: map[string]any{"b": 2, "a": 1, "m": map[string]any{"z": 9, "y": 8}}, Result: "r"},
		},
	}
	first := Generate(snap, fixedNow())
	for i := 0; i < 20; i++ {
		if g := Generate(snap, fixedNow()); g != first {
			t.Fatalf("non-deterministic output on run %d", i)
		}
	}
	// canonical JSON: keys sorted at every level.
	if !strings.Contains(first, "\"a\": 1") || strings.Index(first, "\"a\": 1") > strings.Index(first, "\"b\": 2") {
		t.Errorf("tool Input not key-sorted: %s", first)
	}
	if strings.Index(first, "\"y\": 8") > strings.Index(first, "\"z\": 9") {
		t.Errorf("nested map not key-sorted: %s", first)
	}
}

func TestGenerate_NoTools(t *testing.T) {
	t.Parallel()
	got := Generate(RunSnapshot{Prompt: "q", FinalAnswer: "a", FinishStatus: llm.FinishStop}, fixedNow())
	if !strings.Contains(got, "## 诊断过程\n\n（无工具调用）") {
		t.Errorf("missing no-tool placeholder: %s", got)
	}
}

// TestGenerate_NoFinalAnswer_Abnormal — abnormal termination with no answer
// must render a status-derived note, never an empty 结论 section.
func TestGenerate_NoFinalAnswer_Abnormal(t *testing.T) {
	t.Parallel()
	got := Generate(RunSnapshot{
		Prompt:       "q",
		FinishStatus: llm.FinishError,
		TermCode:     termMaxTurns,
	}, fixedNow())
	if !strings.Contains(got, "- 状态: 达到最大轮数未收敛") {
		t.Errorf("header missing termcode label: %s", got)
	}
	if !strings.Contains(got, "## 结论与建议\n\n（本次诊断未产生最终结论") {
		t.Errorf("conclusion fallback missing: %s", got)
	}
	if strings.Contains(got, "## 结论与建议\n\n\n") {
		t.Errorf("empty conclusion section: %s", got)
	}
}

func TestGenerate_CachedErrorTags(t *testing.T) {
	t.Parallel()
	got := Generate(RunSnapshot{
		Prompt:       "q",
		FinalAnswer:  "a",
		FinishStatus: llm.FinishStop,
		ToolTimeline: []ToolEvent{
			{Name: "c", Result: "r", Cached: true},
			{Name: "e", Result: "boom", IsError: true},
		},
	}, fixedNow())
	if !strings.Contains(got, "### 1. c (cached)") {
		t.Errorf("missing cached tag: %s", got)
	}
	if !strings.Contains(got, "### 2. e (error)") {
		t.Errorf("missing error tag: %s", got)
	}
}

func TestGenerate_EmptyPrompt(t *testing.T) {
	t.Parallel()
	got := Generate(RunSnapshot{FinalAnswer: "a", FinishStatus: llm.FinishStop}, fixedNow())
	if !strings.Contains(got, "## 问题\n\n（无）") {
		t.Errorf("missing empty-prompt placeholder: %s", got)
	}
}

// TestGenerate_Duration renders 耗时 only when both timestamps are set.
func TestGenerate_Duration(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
	snap := RunSnapshot{
		Prompt:       "q",
		FinalAnswer:  "a",
		FinishStatus: llm.FinishStop,
		StartedAt:    start,
		FinishedAt:   start.Add(1500 * time.Millisecond),
	}
	got := Generate(snap, fixedNow())
	if !strings.Contains(got, "- 耗时: 1.5s") {
		t.Errorf("missing/incorrect duration: %s", got)
	}
}
