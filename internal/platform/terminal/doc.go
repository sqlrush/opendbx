// Copyright 2026 opendbx contributors. See LICENSE.
//
// Package terminal probes the host terminal's capabilities (size /
// 24-bit color heuristic / UTF-8 locale / stdin+stdout TTY) and
// surfaces the result as an immutable Capabilities struct.
//
// Design: spec-0.12-tcell-bootstrap § 2.1.
//
// Author: sqlrush
package terminal
