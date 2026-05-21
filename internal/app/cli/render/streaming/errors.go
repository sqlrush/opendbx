// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File errors.go — FinishReason enum + errcode sentinels for streaming
// terminal states. spec-1.6 D-5 (R2 D1 + D7 + D10 errata).
//
// FinishReason classifies why a streaming response terminated. 7 values
// covering normal completion, length cap, tool-use signal, content filter
// rejection, propagated provider error, and context cancellation.
//
// 3 errcode sentinels surface as Close()/Flush() errors:
//   - RENDER.STREAM_TRUNCATED   FinishLength path (token cap reached)
//   - RENDER.STREAM_FILTERED    FinishContentFilter path
//   - RENDER.STREAM_CANCELLED   FinishCancelled path (ctx.Done; R2 D1)
//
// R2 D7: RENDER.STREAM_BUFFER_OVERFLOW deleted — lineBuf cap overflow is
// a renderable state (block.Message.Continued=true) not an error.
//
// FinishStop returns nil error (clean completion).
// FinishToolUse returns nil error (R2 D7: spec-1.6 propagates the signal;
// caller LLM provider switches to tool-use code path).
// FinishError wraps the underlying provider error via Close()/Flush().

package streaming

import "github.com/sqlrush/opendbx/internal/platform/errcode"

// FinishReason classifies why a streaming response terminated.
type FinishReason int

const (
	// FinishUnset means the stream is still in progress.
	FinishUnset FinishReason = iota
	// FinishStop is the normal completion path (LLM returned end-of-output).
	FinishStop
	// FinishLength means the response was truncated by token cap.
	// Surfaces ErrStreamTruncated.
	FinishLength
	// FinishToolUse signals the LLM wants to invoke a tool. spec-1.6 only
	// propagates the signal; caller switches to tool-use path (no error).
	FinishToolUse
	// FinishContentFilter means provider safety filter rejected output.
	// Surfaces ErrStreamFiltered.
	FinishContentFilter
	// FinishError wraps an underlying provider/network error.
	FinishError
	// FinishCancelled means ctx.Done() terminated the stream via Close()
	// without prior finish_reason. Surfaces ErrStreamCancelled. (R2 D1)
	FinishCancelled
)

// String returns the lowercase Anthropic-style finish_reason name.
func (f FinishReason) String() string {
	switch f {
	case FinishUnset:
		return "unset"
	case FinishStop:
		return "stop"
	case FinishLength:
		return "length"
	case FinishToolUse:
		return "tool_use"
	case FinishContentFilter:
		return "content_filter"
	case FinishError:
		return "error"
	case FinishCancelled:
		return "cancelled"
	default:
		return "unknown"
	}
}

// ErrStreamTruncated is returned by Close()/Flush() when the stream
// finished with FinishLength (provider token cap reached). Caller should
// hint the user to split the request or raise max_tokens.
//
// CLAUDE rule 7 三件套 (Code / Message / Hint) per R2 D10.
//
//nolint:gochecknoglobals // spec-0.6 contract: errcode sentinels are package-level.
var ErrStreamTruncated = errcode.Register(
	"RENDER.STREAM_TRUNCATED",
	"LLM stream 输出被 token 限额截断",
	"提示用户拆分请求或增加 max_tokens 上限; 检查 LLM provider 是否支持更高 token 配额",
)

// ErrStreamFiltered is returned by Close()/Flush() when the stream
// finished with FinishContentFilter (provider safety filter rejected).
//
//nolint:gochecknoglobals // spec-0.6 contract: errcode sentinels are package-level.
var ErrStreamFiltered = errcode.Register(
	"RENDER.STREAM_FILTERED",
	"LLM stream 内容被 provider 安全过滤",
	"请用户重新表述请求, 或检查 prompt 是否触发 content filter; 联系 provider 调整过滤策略",
)

// ErrStreamCancelled is returned by Close()/Flush() when the stream was
// terminated via ctx.Done() (caller cancellation or parent ctx timeout)
// without a prior finish_reason event. R2 D1: surfaces FinishCancelled
// state so caller knows the stream was interrupted vs cleanly stopped.
//
//nolint:gochecknoglobals // spec-0.6 contract: errcode sentinels are package-level.
var ErrStreamCancelled = errcode.Register(
	"RENDER.STREAM_CANCELLED",
	"LLM stream 被 context 中断 (用户取消或 parent ctx done)",
	"检查 caller 取消逻辑; 若非预期取消, 重试 stream 调用; 若 parent ctx 是请求级别 timeout 可考虑放宽",
)
