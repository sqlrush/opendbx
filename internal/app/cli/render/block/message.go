// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package block

import "github.com/sqlrush/opendbx/internal/app/cli/render/buffer"

// Message is the spec-0.13 D-3 message block type. Render currently
// returns (nil, ErrUnsupportedNode); spec-1.7+ block-interface 落地
// real Render that uses the fields below.
//
// **spec-1.6 R2 D2 inline errata to spec-0.13 D-3**: 4 fields added by
// spec-1.6 streaming.TokenStream emitter. spec-1.7 block.Message.Render
// MUST inherit these field names and implement the renderer semantics:
//
//   - Text:      raw token content as accumulated by TokenStream. **R2 D6**:
//     contains literal ``` fence markers; spec-1.7 Render
//     identifies + strips fences + applies code-style.
//   - Truncated: TokenStream set this on FinishLength / FinishError /
//     FinishCancelled paths so renderer can show a "…" marker.
//   - Continued: TokenStream set this on lineBuf cap overflow forced emit.
//     **R2 D11 渲染语义**: spec-1.7 Render must NOT draw the
//     trailing newline + show a "…" continuation marker +
//     next block in scrollback is logically continuous.
//   - Empty:     thinking-only path placeholder (痛点 1.5). spec-1.7
//     Render should display "(no output)" or similar.
//
// TODO(spec-1.7): replace stub Render with real implementation respecting
// the 4 fields above.
type Message struct {
	Text      string
	Truncated bool
	Continued bool
	Empty     bool
}

// Render satisfies the RenderNode interface but returns the unsupported
// sentinel for this stub. spec-1.7+ replaces with the real Render that
// consumes the 4 fields above.
func (Message) Render(_ Context) (buffer.Buffer, error) {
	// errcode-lint:exempt -- spec-0.13 D-3: ErrUnsupportedNode is the registered sentinel; spec-1.7+ replaces with real Render.
	return nil, ErrUnsupportedNode
}
