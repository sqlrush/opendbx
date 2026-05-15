// Copyright 2026 opendbx contributors. See LICENSE.
//
// Package tui owns the tcell main event loop.
//
// Design: spec-0.12-tcell-bootstrap (NewScreen factory + Run empty loop;
// goroutine ctx-cancel pathway + IMP-9 tcell-isolation whitelist).
// spec-1.15-tui-program extends with real rendering.
//
// Author: sqlrush
package tui

// Anonymous import keeps gdamore/tcell/v2 in go.sum during T-3 (D-1) before
// T-5 (D-3) lands the real NewScreen factory + Run loop. Removed once
// run.go imports tcell directly.
import _ "github.com/gdamore/tcell/v2"
