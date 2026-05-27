// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File errors.go — LLM.* errcode sentinels (spec-1.20 D-8; 规则 7 三件套).
//
// 原则 3: an unavailable / failed LLM surfaces one of these codes with an
// actionable Hint — opendbx NEVER degrades to a rule-based answer.
// Hints MUST NOT contain the API key value (spec-1.20 R-4).

package llm

import (
	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

// codeRequestInvalid is referenced by RequestInvalidf for detail messages.
const codeRequestInvalid = "LLM.REQUEST_INVALID"

//nolint:gochecknoglobals // spec-0.6 contract: errcode sentinels are package-level.
var (
	// ErrUnavailable — provider construction/connection failed (unknown
	// provider / network / config). 原则 3: no fallback.
	ErrUnavailable = errcode.Register(
		"LLM.UNAVAILABLE",
		"LLM provider 不可用 (未知 provider / 连接失败 / 配置缺失)",
		"检查 config model.provider 是否受支持且网络可达; opendbx 不会降级到规则, LLM 是唯一推理源",
	)
	// ErrAuthFailed — API key missing or invalid.
	ErrAuthFailed = errcode.Register(
		"LLM.AUTH_FAILED",
		"LLM API key 缺失或无效",
		"设置 OPENDBX_LLM_API_KEY 环境变量或 config llm.api_key / models[].api_key; 确认 key 未过期",
	)
	// ErrTimeout — single request timed out (DeadlineExceeded). Multi-turn
	// total timeout is spec-1.21.
	ErrTimeout = errcode.Register(
		"LLM.TIMEOUT",
		"LLM 请求超时",
		"检查网络与 provider 延迟; 可调高 config llm.request_timeout; 复杂诊断总超时在 spec-1.21",
	)
	// ErrCancelled — ctx cancelled (user Ctrl+C / Esc on an in-flight stream).
	ErrCancelled = errcode.Register(
		"LLM.CANCELLED",
		"LLM stream 被用户取消 (Ctrl+C / Esc)",
		"在途输出已保留; 重新输入即可发起新请求",
	)
	// ErrNotImplemented — a config-valid provider/mode not yet built in
	// this stage (openai-compat / ollama / adaptive thinking → spec-3.11).
	ErrNotImplemented = errcode.Register(
		"LLM.NOT_IMPLEMENTED",
		"该 LLM provider / 模式在当前 stage 未实现",
		"adaptive thinking 在 spec-3.11 落地; openai-compat/ollama 已在 spec-1.20.1 落地 (配 model base_url + model)",
	)
	// ErrDecodeFailed — SSE event / tool_use input JSON decode failed.
	ErrDecodeFailed = errcode.Register(
		"LLM.DECODE_FAILED",
		"LLM 响应解码失败 (SSE event / tool_use input JSON)",
		"通常是 provider 协议漂移或 tool input 超出 JSON guard (object/depth/size); 检查 SDK 版本与 tool schema",
	)
	// ErrStreamEmpty — a thinking-only stream ended with no visible content
	// on a non-Stop finish (痛点 1.5 diagnostic aid).
	ErrStreamEmpty = errcode.Register(
		"LLM.STREAM_EMPTY",
		"LLM stream 无可见输出 (thinking-only 且非正常结束)",
		"模型可能仅产 thinking 内容; 检查 thinking budget 与 prompt; 若取消/超时导致, 重试",
	)
	// ErrRequestInvalid — request validation failed (empty Messages /
	// MaxTokens≤0 / system role in Messages / bad thinking budget).
	ErrRequestInvalid = errcode.Register(
		codeRequestInvalid,
		"LLM 请求参数非法",
		"检查 Messages 非空且 role∈{user,assistant} (system 走 Request.System); MaxTokens>0; thinking budget ≥1024 且 < MaxTokens",
	)
	// ErrProviderRefusal — provider returned stop_reason=refusal.
	ErrProviderRefusal = errcode.Register(
		"LLM.PROVIDER_REFUSAL",
		"LLM provider 拒绝生成 (refusal)",
		"请用户重新表述请求; refusal 通常因安全策略, 与 content filter 不同",
	)
	// ErrContentFiltered — OpenAI finish_reason=content_filter (平台内容过滤,
	// 区别于 provider refusal — spec-1.20.1 Q4: FROZEN FinishRefusal 明注
	// "NOT content_filter", 故 content_filter → FinishError + 此码, 不复用 Refusal).
	ErrContentFiltered = errcode.Register(
		"LLM.CONTENT_FILTERED",
		"LLM 平台内容过滤拦截输出 (content_filter)",
		"模型输出被平台安全过滤拦截; 调整 prompt 措辞或换模型; 与 provider refusal 不同",
	)
)

// RequestInvalidf builds an LLM.REQUEST_INVALID error carrying a specific
// detail (errcode.Newf inherits the registered Hint). Used by
// ValidateRequest so each validation branch reports its precise cause.
func RequestInvalidf(detail string) error {
	return errcode.Newf(codeRequestInvalid, "LLM 请求参数非法: %s", detail)
}
