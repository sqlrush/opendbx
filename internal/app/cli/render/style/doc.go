// Copyright 2026 opendbx contributors. See LICENSE.
//
// Package style defines visual attributes (FG/BG color + Bold/Italic/
// Underline/Reverse) attached to render/buffer cells. Generates SGR
// ANSI escape sequences for render/terminal driver flush.
//
// Invariant (CLAUDE.md § 3.1 / AD-002): ANSI() output is consumed ONLY
// by render/terminal; other render subpackages must use Style as data,
// never call ANSI() to write directly to stdout.
//
// DAG position: render/style is index 1 (leaf).
//
// Design: spec-0.13-render-engine-skeleton § 2.1 (D-1)
// Author: sqlrush
package style
