// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File stream.go — production TokenStream impl. spec-1.6 D-1 + D-2 + D-3
// + D-4 + D-6 (R2 + R2.1 + R2.2 errata batch).
//
// Public surface (spec-1.6 R2 D1+D3 / R2.2 修正 1+2):
//
//	NewTokenStream(ctx, opts...) *TokenStream
//	WithChanCapacity(n) Option   // default 512
//	WithLineBufferCap(n) Option  // default 64KB
//	WithFinishHook(fn) Option
//
//	(*TokenStream) AppendChunk(c Chunk) error          // multi-producer
//	(*TokenStream) Append(token string)                // spec-0.13 D-1 legacy
//	(*TokenStream) Drain() []block.RenderNode          // main-loop only
//	(*TokenStream) Flush() (block.RenderNode, error)   // spec-0.13 D-1 legacy
//	(*TokenStream) Close() error                       // cancel + flush partial
//	(*TokenStream) FinishReason() FinishReason
//
// Concurrency contract (R2 D5 matrix; R2.2 ownership 修正):
//   - Drain is main-goroutine only (spec-1.4 RenderFn CRIT-A inherit).
//   - AppendChunk safe from any goroutine (multi-producer).
//   - Close / Flush / FinishReason safe from any goroutine.
//   - Internal sync.Mutex serializes all state mutations.
//
// Close-Drain ownership (R2 D3 + R2.2 修正 2):
//   - Close drains pending chunks first, then force-flushes lineBuf to
//     s.emitted, then returns errForFinishUnsafe. Does NOT consume emitted.
//   - Caller MUST call Drain() after Close to collect final blocks
//     (thinking-only Empty placeholder; FinishCancelled partial Truncated;
//     FinishLength truncated marker).
//
// Fence integral emit (R2 D6 + R2.2 修正 1):
//   - openFence=true accumulates lineBuf without emit.
//   - Fence close (open→close transition) emits whole lineBuf as ONE
//     block.Message{Text: "```lang\n...\n```"} preserving raw markers.
//   - openFence=false uses tryEmitLines to split by '\n' (one block per line).
//   - spec-1.7 block.Message.Render identifies fence blocks via Text
//     prefix/suffix backticks; no cross-block state machine needed.

package streaming

import (
	"bytes"
	"context"
	"sync"

	"github.com/sqlrush/opendbx/internal/app/cli/render/block"
)

const (
	defaultChanCap    = 512
	defaultLineBufCap = 64 * 1024
)

// Chunk is one streaming event from a producer (LLM HTTP layer / worker).
type Chunk struct {
	Token        string       // partial text; "" allowed (thinking mode)
	FinishReason FinishReason // FinishUnset for in-progress
	Err          error        // non-nil → FinishError path
}

// Option configures NewTokenStream via the functional options pattern.
type Option func(*streamOptions)

type streamOptions struct {
	chanCap    int
	lineBufCap int
	onFinish   func(FinishReason)
}

// WithChanCapacity sets the token chan buffer size (default 512).
func WithChanCapacity(n int) Option { return func(o *streamOptions) { o.chanCap = n } }

// WithLineBufferCap sets the partial-line buffer cap in bytes (default 64KB).
// Overflow triggers forced emit with Continued=true marker per R2 D11.
func WithLineBufferCap(n int) Option { return func(o *streamOptions) { o.lineBufCap = n } }

// WithFinishHook installs an optional callback fired when any finish_reason
// is recorded (length / stop / tool_use / etc).
func WithFinishHook(fn func(FinishReason)) Option {
	return func(o *streamOptions) { o.onFinish = fn }
}

// TokenStream is the production Stream impl: multi-producer + single-
// consumer + partial-line accumulator + open-fence integral emit +
// finish_reason propagation (痛点 1.1 防御).
type TokenStream struct {
	mu                 sync.Mutex
	tokens             chan Chunk
	lineBuf            bytes.Buffer
	emitted            []block.RenderNode
	openFence          bool
	pendingBacktickRun int // R2 D8: cross-chunk backtick run length
	finish             FinishReason
	lastErr            error
	ctx                context.Context
	cancel             context.CancelFunc
	opts               streamOptions
	sawContent         bool // 痛点 1.5: thinking-only detection
	closed             bool
}

// Compile-time assert that *TokenStream satisfies spec-0.13 D-1 Stream.
var _ Stream = (*TokenStream)(nil)

// NewTokenStream constructs a TokenStream bound to ctx; cancellation
// propagates through derived ctx. Defaults: chanCap=512, lineBufCap=64KB.
func NewTokenStream(ctx context.Context, opts ...Option) *TokenStream {
	o := streamOptions{chanCap: defaultChanCap, lineBufCap: defaultLineBufCap}
	for _, opt := range opts {
		opt(&o)
	}
	sctx, cancel := context.WithCancel(ctx)
	return &TokenStream{
		tokens: make(chan Chunk, o.chanCap),
		ctx:    sctx,
		cancel: cancel,
		opts:   o,
	}
}

// AppendChunk submits one chunk. Blocking backpressure with ctx escape:
// select between ctx.Done() and tokens<-c; if chan is full and ctx is
// not done, blocks until consumer drains or ctx cancels.
//
// Safe from any goroutine (multi-producer). Returns ErrStreamCancelled
// if the stream is already closed or ctx is done.
//
// 痛点 1.1 防御: callers MUST pass finish_reason via this entry — Close
// does not invent finish_reason from absence.
func (s *TokenStream) AppendChunk(c Chunk) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrStreamCancelled
	}
	s.mu.Unlock()

	// Non-blocking ctx priority check (Go's select picks randomly among
	// ready cases; this ensures ctx.Done() takes priority over a non-full
	// tokens chan).
	select {
	case <-s.ctx.Done():
		return ErrStreamCancelled
	default:
	}

	select {
	case <-s.ctx.Done():
		return ErrStreamCancelled
	case s.tokens <- c:
		return nil
	}
}

// Append is the spec-0.13 D-1 legacy entry. Thin wrapper around
// AppendChunk(Chunk{Token: token}). Callers needing finish_reason MUST
// use AppendChunk directly; Append paths terminate via Close.
func (s *TokenStream) Append(token string) {
	_ = s.AppendChunk(Chunk{Token: token})
}

// Drain pulls every buffered chunk through processChunkUnsafe, then
// returns the accumulated block.RenderNodes ready for caller.Push.
// Caller takes ownership of the returned slice; subsequent Drain calls
// return a fresh slice (or nil).
//
// **Main-loop only** (spec-1.4 RenderFn CRIT-A inherit). Internal Lock
// is defense-in-depth — a misuse from a worker goroutine will not race.
func (s *TokenStream) Drain() []block.RenderNode {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.drainChunksUnsafe()
	out := s.emitted
	s.emitted = nil
	return out
}

// drainChunksUnsafe — caller holds s.mu. Processes every pending chunk
// in s.tokens via processChunkUnsafe; does NOT consume or clear s.emitted.
// Shared by Drain / Close / Flush per R2.2 修正 2 (avoid pending-after-final
// ordering bug).
func (s *TokenStream) drainChunksUnsafe() {
	for {
		select {
		case c := <-s.tokens:
			s.processChunkUnsafe(c)
		default:
			return
		}
	}
}

// processChunkUnsafe — caller holds s.mu.
//
// **R2 D5 (HIGH-E)**: Err path processes c.Token first (Anthropic SSE
// error event may carry partial content) before setting finish + flush
// + propagate.
//
// **R2.2 修正 1 (fence 整段 emit)**: scanFenceStateUnsafe updates
// s.openFence; if prevOpenFence && !s.openFence (close transition),
// emit whole lineBuf as ONE block.Message preserving raw fence markers.
// Normal text path (openFence=false) uses tryEmitLinesUnsafe per-newline.
func (s *TokenStream) processChunkUnsafe(c Chunk) {
	if c.Token != "" {
		s.sawContent = true
		s.lineBuf.WriteString(c.Token)
		_, closed := s.scanFenceStateUnsafe(c.Token)

		switch {
		case closed:
			// Any close transition (open→close, including open+close within
			// same token) → emit whole lineBuf as 1 block preserving raw
			// fence markers per R2 D6 + R2.2 修正 1.
			s.emitWholeBufferUnsafe(false /*truncated*/, false /*continued*/)
		case !s.openFence:
			// No transitions + not in fence → normal line emit.
			s.tryEmitLinesUnsafe()
		}
		// openFence=true (fence still open): accumulate, cap check only.
		if s.openFence && s.lineBuf.Len() > s.opts.lineBufCap {
			s.emitWholeBufferUnsafe(false /*truncated*/, true /*continued*/)
		}
	}

	if c.Err != nil {
		s.finish = FinishError
		s.lastErr = c.Err
		s.flushPartialUnsafe(true /*truncated*/)
		if s.opts.onFinish != nil {
			s.opts.onFinish(FinishError)
		}
		return
	}

	if c.FinishReason != FinishUnset {
		s.finish = c.FinishReason
		s.flushPartialUnsafe(c.FinishReason == FinishLength)
		if s.opts.onFinish != nil {
			s.opts.onFinish(c.FinishReason)
		}
	}
}

// scanFenceStateUnsafe (R2 D8 + R2.2 propagate) — state-machine fence
// tracker handling cross-chunk delimiters. Maintains pendingBacktickRun
// across chunks; flips openFence when a run of >= 3 backticks is followed
// by a non-backtick byte (or end of token while in run-collection state).
// 1-2 backticks (inline code) reset run without flipping.
//
// Returns (opened, closed) flags indicating whether any false→true or
// true→false transition occurred during this token. Caller uses these
// to decide between integral emit (closed) vs accumulate (opened) vs
// normal line emit (neither).
func (s *TokenStream) scanFenceStateUnsafe(token string) (opened, closed bool) {
	flip := func() {
		prev := s.openFence
		s.openFence = !s.openFence
		if !prev && s.openFence {
			opened = true
		}
		if prev && !s.openFence {
			closed = true
		}
	}
	for i := 0; i < len(token); i++ {
		if token[i] == '`' {
			s.pendingBacktickRun++
			continue
		}
		if s.pendingBacktickRun >= 3 {
			flip()
		}
		s.pendingBacktickRun = 0
	}
	if s.pendingBacktickRun >= 3 {
		flip()
		s.pendingBacktickRun = 0
	}
	return
}

// tryEmitLinesUnsafe (R2.2 修正 1) — split lineBuf by '\n', emit each
// completed line as one block.Message. Only called when openFence=false.
// Cap check on the partial tail (line without '\n') triggers forced emit.
func (s *TokenStream) tryEmitLinesUnsafe() {
	data := s.lineBuf.Bytes()
	for {
		idx := bytes.IndexByte(data, '\n')
		if idx < 0 {
			break
		}
		// Copy to avoid aliasing the buffer's backing array.
		line := make([]byte, idx)
		copy(line, data[:idx])
		s.emitted = append(s.emitted, block.Message{Text: string(line)})
		data = data[idx+1:]
	}
	// Retain partial tail.
	s.lineBuf.Reset()
	s.lineBuf.Write(data)
	if s.lineBuf.Len() > s.opts.lineBufCap {
		s.emitWholeBufferUnsafe(false /*truncated*/, true /*continued*/)
	}
}

// emitWholeBufferUnsafe (R2.2 修正 1) — emit current lineBuf as ONE
// block.Message preserving raw fence markers (per R2 D6). Resets lineBuf.
// Used by: fence close transition; cap overflow; flushPartialUnsafe final.
func (s *TokenStream) emitWholeBufferUnsafe(truncated, continued bool) {
	if s.lineBuf.Len() == 0 {
		return
	}
	s.emitted = append(s.emitted, block.Message{
		Text:      s.lineBuf.String(),
		Truncated: truncated,
		Continued: continued,
	})
	s.lineBuf.Reset()
}

// flushPartialUnsafe — emit lineBuf as final block on stream finish.
// **R2.2**: uses emitWholeBufferUnsafe (even if openFence=true the whole
// buffer is emitted as 1 block; half-finished fence is truncated path).
// Also handles 痛点 1.5 thinking-only path (Empty placeholder block).
func (s *TokenStream) flushPartialUnsafe(truncated bool) {
	if s.lineBuf.Len() > 0 {
		s.emitWholeBufferUnsafe(truncated, false /*not continued*/)
	}
	if !s.sawContent && s.finish == FinishStop {
		s.emitted = append(s.emitted, block.Message{Empty: true})
	}
}

// Close cancels the derived ctx, processes any pending chunks, and
// force-flushes lineBuf to s.emitted as a final block.
//
// **Idempotent**: subsequent Close calls are no-op returning nil.
//
// **R2 D3 (HIGH-C+F) ownership**: Close does NOT consume s.emitted.
// Caller MUST call Drain() after Close to collect final blocks
// (thinking-only Empty placeholder; FinishCancelled partial Truncated;
// FinishLength truncated marker).
//
// **R2 D1 (HIGH-A) ctx cancel surface**: if no prior finish_reason was
// set, records s.finish=FinishCancelled + s.lastErr=ErrStreamCancelled
// so errForFinishUnsafe surfaces the cancellation.
//
// **R2.2 修正 2 ordering**: drainChunksUnsafe first (process pending so
// pending chunk blocks appear before the final partial in the caller's
// next Drain), then flushPartialUnsafe.
func (s *TokenStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	s.cancel()
	// R2.2: process pending chunks BEFORE recording cancellation state.
	// Pending chunks may carry an explicit FinishReason / Err that should
	// take priority over implicit ctx-cancel.
	s.drainChunksUnsafe()
	if s.finish == FinishUnset {
		s.finish = FinishCancelled
		s.lastErr = ErrStreamCancelled
	}
	// R2.2 fix: only flush if lineBuf has content. The thinking-only
	// Empty placeholder path is fully handled by processChunkUnsafe via
	// the FinishStop chunk; re-running flushPartialUnsafe here would
	// emit a duplicate Empty block.
	if s.lineBuf.Len() > 0 {
		s.flushPartialUnsafe(
			s.finish == FinishLength ||
				s.finish == FinishCancelled ||
				s.finish == FinishError,
		)
	}
	// errcode-lint:exempt -- spec-1.6 D-5: errForFinishUnsafe returns one of the registered errcode sentinels (ErrStreamTruncated/Filtered/Cancelled) or the underlying Chunk.Err (already from provider boundary). Wrapping again would shadow the sentinel.
	return s.errForFinishUnsafe()
}

// Flush is the spec-0.13 D-1 legacy entry. **R2 D3 + R2.2 fix**:
// drainChunksUnsafe → flushPartialUnsafe → consume + clear s.emitted →
// return last block + finish-state error. Ensures legacy
// `Append("hello"); Flush()` returns a non-nil block per spec-0.13 D-1.
func (s *TokenStream) Flush() (block.RenderNode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.drainChunksUnsafe()
	if s.lineBuf.Len() > 0 {
		s.flushPartialUnsafe(
			s.finish == FinishLength ||
				s.finish == FinishCancelled ||
				s.finish == FinishError,
		)
	}
	// errcode-lint:exempt -- spec-1.6 D-5: errForFinishUnsafe returns a registered errcode sentinel or the underlying provider error from Chunk.Err.
	err := s.errForFinishUnsafe()
	blocks := s.emitted
	s.emitted = nil
	if len(blocks) == 0 {
		return nil, err
	}
	return blocks[len(blocks)-1], err
}

// FinishReason returns the recorded stream end reason. Safe from any
// goroutine.
func (s *TokenStream) FinishReason() FinishReason {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.finish
}

// errForFinishUnsafe — caller holds s.mu. Maps finish state to error
// sentinel (R2 D1: includes FinishCancelled case).
func (s *TokenStream) errForFinishUnsafe() error {
	switch s.finish {
	case FinishLength:
		return ErrStreamTruncated
	case FinishContentFilter:
		return ErrStreamFiltered
	case FinishCancelled:
		return ErrStreamCancelled
	case FinishError:
		return s.lastErr
	}
	return nil
}
