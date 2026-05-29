// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package fake

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sqlrush/opendbx/internal/domain/llm"
)

// Provider is a scripted llm.Provider with two modes:
//
//   - Single-turn (legacy spec-1.20 D-3): every Stream call replays the
//     same `script` chunks. Built via New / Scripted.
//   - Multi-turn (spec-1.21 D-7): each Stream call consumes the next
//     entry of `turns`; running past the end yields a registered
//     REQUEST_INVALID errcode rather than silently re-running the last
//     turn (spec D-7: "callIdx >= len(turns) → 注册 error;
//     跑过脚本 = 测试 bug, 非静默重放"). Built via NewScriptedTurns.
//
// The two modes are mutually exclusive — a single Provider instance is
// either flat-script or turns-based, never both.
type Provider struct {
	script   []llm.Chunk   // single-turn mode (nil when turns mode active)
	turns    [][]llm.Chunk // multi-turn mode (nil when flat mode active)
	delay    time.Duration // inter-chunk delay (0 = immediate)
	startErr error         // non-nil → Stream returns this immediately
	mu       sync.Mutex    // guards callIdx
	callIdx  int           // multi-turn cursor; advanced by Stream
}

// Compile-time Provider satisfaction.
var _ llm.Provider = (*Provider)(nil)

// New constructs a fake Provider replaying the given chunks.
func New(chunks ...llm.Chunk) *Provider {
	return &Provider{script: chunks}
}

// WithDelay sets an inter-chunk delay (simulates streaming pacing).
func (p *Provider) WithDelay(d time.Duration) *Provider {
	p.delay = d
	return p
}

// WithStartErr makes Stream return err immediately (no iterator) — for
// testing the immediate-error path (LLM.AUTH_FAILED / REQUEST_INVALID /
// UNAVAILABLE / connect-time ctx.Canceled).
func (p *Provider) WithStartErr(err error) *Provider {
	p.startErr = err
	return p
}

// Scripted builds a provider that streams text split into one-rune-ish
// chunks then terminates with finish. Convenience for the common case.
func Scripted(text string, finish llm.FinishReason) *Provider {
	var chunks []llm.Chunk
	for _, r := range text {
		chunks = append(chunks, llm.Chunk{Token: string(r)})
	}
	chunks = append(chunks, llm.Chunk{FinishReason: finish})
	return New(chunks...)
}

// Turn is one scripted assistant turn in multi-turn mode (spec-1.21 D-7).
// Text → one Token chunk (assistant prose; provider may further split
// this in real impls, but a single chunk is sufficient for the loop
// state-machine tests this fake serves). ToolUses + Finish ride the
// terminal chunk: a turn with ToolUses + Finish=FinishToolUse drives the
// Loop's tool dispatch path; a bare Finish=FinishStop terminates.
type Turn struct {
	Text     string
	ToolUses []llm.ToolUse
	Finish   llm.FinishReason
}

// NewScriptedTurns builds a multi-turn fake. The Nth Stream call replays
// turns[N-1]. After len(turns) calls the provider returns
// LLM.REQUEST_INVALID per spec-1.21 D-7 — silently re-running the last
// turn would mask "test ran longer than scripted" bugs.
func NewScriptedTurns(turns ...Turn) *Provider {
	scripts := make([][]llm.Chunk, 0, len(turns))
	for _, t := range turns {
		scripts = append(scripts, turnChunks(t))
	}
	return &Provider{turns: scripts}
}

// turnChunks renders one Turn into the chunk sequence a Stream emits:
// optional text chunk first, then a terminal chunk carrying Finish +
// any ToolUses.
func turnChunks(t Turn) []llm.Chunk {
	out := make([]llm.Chunk, 0, 2)
	if t.Text != "" {
		out = append(out, llm.Chunk{Token: t.Text})
	}
	out = append(out, llm.Chunk{FinishReason: t.Finish, ToolUses: t.ToolUses})
	return out
}

// Name implements llm.Provider.
func (p *Provider) Name() string { return "fake" }

// Stream implements llm.Provider. Returns startErr immediately if set;
// otherwise validates the request and returns a scripted iterator. In
// multi-turn mode each call advances the turn cursor (spec-1.21 D-7).
func (p *Provider) Stream(ctx context.Context, req llm.Request) (llm.Stream, error) {
	// errcode-lint:exempt -- spec-1.20 D-3/D-8: startErr is a caller-supplied registered LLM.* sentinel (WithStartErr injects e.g. ErrAuthFailed/ErrUnavailable for the no-key bootstrap path); pass-through, not a fresh error origin.
	if p.startErr != nil {
		return nil, p.startErr
	}
	// errcode-lint:exempt -- spec-1.20 D-8: ValidateRequest returns REQUEST_INVALID (registered errcode); pass-through.
	if err := llm.ValidateRequest(req); err != nil {
		return nil, err
	}
	script := p.script
	if p.turns != nil {
		p.mu.Lock()
		idx := p.callIdx
		p.callIdx++
		p.mu.Unlock()
		if idx >= len(p.turns) {
			// errcode-lint:exempt -- spec-1.21 D-7: RequestInvalidf returns LLM.REQUEST_INVALID (registered); script exhaustion = test bug, surface explicitly.
			return nil, llm.RequestInvalidf(fmt.Sprintf("fake: NewScriptedTurns ran out of scripted turns at call %d (test scripted fewer turns than the loop consumed)", idx+1))
		}
		script = p.turns[idx]
	}
	return &stream{ctx: ctx, script: script, delay: p.delay}, nil
}

// CallCount reports how many Stream invocations the provider has served
// — useful for tests asserting the loop did the expected number of
// round-trips. Multi-turn mode only; flat-mode value is always 0.
func (p *Provider) CallCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.callIdx
}

// stream is the scripted llm.Stream iterator.
type stream struct {
	ctx    context.Context
	script []llm.Chunk
	delay  time.Duration
	idx    int
	cur    llm.Chunk
	err    error
}

// Next advances to the next scripted chunk, honoring ctx cancellation.
func (s *stream) Next() bool {
	if s.err != nil || s.idx >= len(s.script) {
		return false
	}
	if s.delay > 0 {
		// T-10a LOW-2: NewTimer + Stop so the timer goroutine does not
		// outlive a ctx-cancel win (time.After leaks until it fires).
		timer := time.NewTimer(s.delay)
		select {
		case <-s.ctx.Done():
			timer.Stop()
			s.err = s.ctx.Err()
			return false
		case <-timer.C:
		}
	} else {
		select {
		case <-s.ctx.Done():
			s.err = s.ctx.Err()
			return false
		default:
		}
	}
	s.cur = s.script[s.idx]
	s.idx++
	return true
}

// Chunk returns the current chunk.
func (s *stream) Chunk() llm.Chunk { return s.cur }

// Err returns the terminal error (ctx.Err on cancellation; else nil).
func (s *stream) Err() error { return s.err }

// Close is a no-op for the fake stream (idempotent).
func (s *stream) Close() error { return nil }
