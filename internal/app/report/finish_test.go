// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package report

import (
	"strings"
	"testing"

	"github.com/sqlrush/opendbx/internal/app/diagnose"
	"github.com/sqlrush/opendbx/internal/domain/llm"
)

// TestFinishLabel_AllReasons — every llm.FinishReason maps to a non-empty,
// non-"未知" label (codex M-1: exhaustive termination coverage).
func TestFinishLabel_AllReasons(t *testing.T) {
	t.Parallel()
	all := []llm.FinishReason{
		llm.FinishUnset, llm.FinishStop, llm.FinishLength, llm.FinishToolUse,
		llm.FinishStopSequence, llm.FinishPause, llm.FinishRefusal,
		llm.FinishCancelled, llm.FinishError,
	}
	for _, fr := range all {
		lbl := finishLabel(fr, "")
		if lbl == "" {
			t.Errorf("FinishReason %v → empty label", fr)
		}
		if lbl == "未知状态" {
			t.Errorf("FinishReason %v fell through to default — add an explicit case", fr)
		}
	}
}

func TestFinishLabel_AllTermCodes(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		termMaxTurns:        "达到最大轮数未收敛",
		termTotalTimeout:    "总诊断超时",
		termToolUnknown:     "调用了未知工具",
		termToolTimeout:     "工具执行超时",
		termUnexpectedPause: "意外暂停",
	}
	for code, want := range cases {
		if got := finishLabel(llm.FinishError, code); got != want {
			t.Errorf("finishLabel(_, %q) = %q; want %q", code, got, want)
		}
	}
	// Unknown termcode → graceful fallback carrying the raw code.
	if got := finishLabel(llm.FinishError, "DIAGNOSE.SOMETHING_NEW"); !strings.Contains(got, "DIAGNOSE.SOMETHING_NEW") {
		t.Errorf("unknown termcode fallback bad: %q", got)
	}
}

// TestTermCodes_MatchDiagnose pins the report's term-code constants against the
// real diagnose.Err*.Code() values — drift guard so a renamed code in diagnose
// surfaces here instead of silently degrading the report (architect M-1).
func TestTermCodes_MatchDiagnose(t *testing.T) {
	t.Parallel()
	pairs := []struct {
		local string
		code  string
	}{
		{termMaxTurns, diagnose.ErrMaxTurns.Code()},
		{termTotalTimeout, diagnose.ErrTotalTimeout.Code()},
		{termToolUnknown, diagnose.ErrToolUnknown.Code()},
		{termToolTimeout, diagnose.ErrToolTimeout.Code()},
		{termUnexpectedPause, diagnose.ErrUnexpectedPause.Code()},
	}
	for _, p := range pairs {
		if p.local != p.code {
			t.Errorf("term code drift: report has %q, diagnose has %q", p.local, p.code)
		}
	}
}

func TestNoAnswerNote(t *testing.T) {
	t.Parallel()
	note := noAnswerNote(llm.FinishError, termTotalTimeout)
	if !strings.Contains(note, "未产生最终结论") || !strings.Contains(note, "总诊断超时") {
		t.Errorf("no-answer note bad: %q", note)
	}
}
