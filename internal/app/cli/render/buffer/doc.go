// Copyright 2026 opendbx contributors. See LICENSE.
//
// Package buffer — spec-0.13 D-1 interface skeleton + spec-1.2
// cell-grid-buffer primary implementation.
//
// See buffer.go for the Cell + Buffer interface (spec-0.13 D-3 FROZEN),
// grid.go for the concrete *Grid (D-1 row-major flat array), pool.go for
// BufferPool 3-bucket sync.Pool (D-2), reset.go for O(1) generational
// reset (D-3), and wide.go for the WideContinuation rune sentinel +
// IsContinuation public API (D-4).
//
// Design: spec-0.13-render-engine-skeleton § 2.1 (D-1, interface skeleton);
// spec-1.2-cell-grid-buffer (primary implementation reference).
// Author: sqlrush
package buffer
