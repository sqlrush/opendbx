// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File finish.go — exhaustive termination-status → label mapping (spec-1.23
// D-1). Covers all 9 llm.FinishReason values plus the 5 DIAGNOSE.* TermCodes,
// so the report Header and the no-answer conclusion are well-defined for every
// terminal state (codex M-1 / architect M-1).

package report

import "github.com/sqlrush/opendbx/internal/domain/llm"

// DIAGNOSE.* term codes. These mirror diagnose.Err*.Code(); a test
// (TestTermCodes_MatchDiagnose) imports diagnose and asserts no drift, so the
// report package stays decoupled from the diagnose error registry while still
// being verified against it.
const (
	termMaxTurns        = "DIAGNOSE.MAX_TURNS"
	termTotalTimeout    = "DIAGNOSE.TOTAL_TIMEOUT"
	termToolUnknown     = "DIAGNOSE.TOOL_UNKNOWN"
	termToolTimeout     = "DIAGNOSE.TOOL_TIMEOUT"
	termUnexpectedPause = "DIAGNOSE.UNEXPECTED_PAUSE"
)

// finishLabel maps a terminal state to a human label. A non-empty TermCode
// (a DIAGNOSE.* orchestration termination) takes precedence over the raw
// FinishReason.
func finishLabel(fr llm.FinishReason, termCode string) string {
	if termCode != "" {
		if lbl := termCodeLabel(termCode); lbl != "" {
			return lbl
		}
		return "异常终止 (" + termCode + ")"
	}
	switch fr {
	case llm.FinishStop, llm.FinishStopSequence:
		return "正常完成"
	case llm.FinishLength:
		return "达到长度上限（截断）"
	case llm.FinishRefusal:
		return "模型拒绝"
	case llm.FinishCancelled:
		return "用户取消"
	case llm.FinishError:
		return "错误终止"
	case llm.FinishPause:
		return "意外暂停"
	case llm.FinishToolUse:
		return "工具调用未收敛"
	case llm.FinishUnset:
		return "未完成"
	default:
		return "未知状态"
	}
}

func termCodeLabel(code string) string {
	switch code {
	case termMaxTurns:
		return "达到最大轮数未收敛"
	case termTotalTimeout:
		return "总诊断超时"
	case termToolUnknown:
		return "调用了未知工具"
	case termToolTimeout:
		return "工具执行超时"
	case termUnexpectedPause:
		return "意外暂停"
	default:
		return ""
	}
}

// noAnswerNote is the conclusion-section fallback when a run produced no final
// user-visible answer (abnormal termination, or a run that ended mid-tool-use).
func noAnswerNote(fr llm.FinishReason, termCode string) string {
	return "（本次诊断未产生最终结论；终止状态：" + finishLabel(fr, termCode) + "。可重试或简化问题。）"
}
