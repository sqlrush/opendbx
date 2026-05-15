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
