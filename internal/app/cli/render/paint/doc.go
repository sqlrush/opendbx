// Copyright 2026 opendbx contributors. See LICENSE.

// Package paint provides cell-to-cell blit helpers that enforce the
// spec-1.7 T-9 HIGH-2 continuation contract — namely, that wide-rune
// continuation cells from a source buffer MUST NOT be re-written to
// the destination, because dst.SetCell on a wide-main auto-writes its
// own continuation; writing the continuation again triggers
// clearWideOverlap and erases the wide-main at (x-1).
//
// Spec: opendbrb/specs/stage-1/spec-1.20.2-reasoning-render.md (D-1)
// Author: sqlrush
//
// Origin: spec-1.20.1 user-terminal evidence found this pattern duplicated
// across five sites — llmapp/model.go paintBufferAt, program/program.go
// paintBufferAt, scrollback/virtual.go composeInto, scrollback/cache.go
// copyCells, and block/message.go stitched (already fixed in spec-1.7
// T-9 HIGH-2). Each missed instance shredded CJK / markdown output on
// real terminals. spec-1.20.2 centralizes the pattern here so the
// contract is enforced in exactly one place, and a companion lint
// (tools/paint-pattern-lint) rejects future bare SetCell(src.Cell())
// drift.
//
// DAG (§ 3.1): paint sits at index 3.5 between buffer (3) and layout (4).
// It depends only on buffer (for IsContinuation + Grid + Cell) and is
// imported by block / scrollback / non-render consumers (llmapp, program).
package paint
