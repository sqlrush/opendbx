// Copyright 2026 opendbx contributors. See LICENSE.
//
// Stage 0 stub: prints "not yet implemented" message.
// Real interactive TUI dispatch lands via internal/entrypoints in spec-0.3.
//
// Design: opendbrb/specs/stage-0/spec-0.2-go-module-layout.md D-2.
// Author: sqlrush
package main

import (
	"fmt"
	"io"
)

func runInteract(_ []string, stdout, _ io.Writer) int {
	fmt.Fprintf(stdout, stage0StubFmt,
		"interact",
		"interact",
		"spec-0.3-cmd-entrypoints + spec-1.15-tui + spec-1.16-input-three-modes")
	return 0
}
