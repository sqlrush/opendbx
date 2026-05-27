// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package fake

import (
	"context"
	"time"

	"github.com/sqlrush/opendbx/internal/domain/llm"
)

// Provider is a scripted llm.Provider. It replays a fixed Chunk sequence,
// honoring ctx cancellation between chunks (spec-1.20 D-3).
type Provider struct {
	script   []llm.Chunk
	delay    time.Duration // inter-chunk delay (0 = immediate)
	startErr error         // non-nil → Stream returns this immediately (auth/invalid/unavailable test)
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

// Name implements llm.Provider.
func (p *Provider) Name() string { return "fake" }

// Stream implements llm.Provider. Returns startErr immediately if set;
// otherwise validates the request and returns a scripted iterator.
func (p *Provider) Stream(ctx context.Context, req llm.Request) (llm.Stream, error) {
	// errcode-lint:exempt -- spec-1.20 D-3/D-8: startErr is a caller-supplied registered LLM.* sentinel (WithStartErr injects e.g. ErrAuthFailed/ErrUnavailable for the no-key bootstrap path); pass-through, not a fresh error origin.
	if p.startErr != nil {
		return nil, p.startErr
	}
	// errcode-lint:exempt -- spec-1.20 D-8: ValidateRequest returns REQUEST_INVALID (registered errcode); pass-through.
	if err := llm.ValidateRequest(req); err != nil {
		return nil, err
	}
	return &stream{ctx: ctx, script: p.script, delay: p.delay}, nil
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
