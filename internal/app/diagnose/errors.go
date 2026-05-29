// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File errors.go — DIAGNOSE.* errcode sentinels (spec-1.21 D-5; 规则 7
// 三件套). Two termination-vs-feedback classes:
//
//   - Terminal (Loop.Run returns error + Result.TermCode):
//     MAX_TURNS / TOTAL_TIMEOUT / TOOL_UNKNOWN / UNEXPECTED_PAUSE
//   - Feedback (written into ToolResult.Content, loop continues):
//     TOOL_TIMEOUT (LLM self-corrects on the next turn)
//
// LLM.TIMEOUT (per-request, provider layer) and DIAGNOSE.TOTAL_TIMEOUT
// (orchestration layer) coexist across layers (per-turn LLM timeout →
// LLM.TIMEOUT; total loop timeout → DIAGNOSE.TOTAL_TIMEOUT).

package diagnose

import (
	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

//nolint:gochecknoglobals // spec-0.6 contract: errcode sentinels are package-level.
var (
	// ErrMaxTurns — loop hit the configured max-turns ceiling without a
	// natural FinishStop. Terminal class.
	ErrMaxTurns = errcode.Register(
		"DIAGNOSE.MAX_TURNS",
		"诊断轮数达上限未收敛",
		"提高 diagnose.max_turns 或简化问题; 检查工具反复调用 (spec-1.22 去重)",
	)
	// ErrTotalTimeout — orchestration-level total-diagnose deadline hit.
	// Distinct from LLM.TIMEOUT (per-request, provider layer). Terminal.
	ErrTotalTimeout = errcode.Register(
		"DIAGNOSE.TOTAL_TIMEOUT",
		"诊断总时长超限 (默认 10min)",
		"提高 diagnose.total_timeout 或缩小诊断范围",
	)
	// ErrToolUnknown — model emitted a tool_use referencing a name absent
	// from the executor registry. Terminal — do NOT append the orphan
	// assistant turn (防 Anthropic 400 on resume; T-2 HIGH-5).
	ErrToolUnknown = errcode.Register(
		"DIAGNOSE.TOOL_UNKNOWN",
		"模型请求未注册的工具",
		"tool_use 指向未知工具; 检查注入 Tools schema 与 registry 一致",
	)
	// ErrToolTimeout — single tool exceeded per-tool deadline (default 30s).
	// Feedback class — written into ToolResult.Content with IsError=true;
	// the model self-corrects on the next turn. NOT terminal.
	ErrToolTimeout = errcode.Register(
		"DIAGNOSE.TOOL_TIMEOUT",
		"单轮工具执行超时 (默认 30s)",
		"检查工具/目标 DB 响应; 提高 diagnose.tool_timeout",
	)
	// ErrUnexpectedPause — provider returned FinishPause but spec-1.21 has
	// no server-side tool wired. Terminal (defensive; signals SDK/model
	// behavior drift rather than a normal control flow).
	ErrUnexpectedPause = errcode.Register(
		"DIAGNOSE.UNEXPECTED_PAUSE",
		"收到非预期 pause_turn (spec-1.21 未启用 server tool)",
		"server-side tool 是未来 spec; 检查 model/请求未启用 server tool",
	)
)
