// Copyright 2026 opendbx contributors. See LICENSE.
//
// Stage 0 stub: autopilot agent mode. Lands in Stage 9+.
//
// Design: opendbrb/specs/stage-0/spec-0.2-go-module-layout.md D-2.
// Author: sqlrush
package main

import (
	"fmt"
	"io"
)

func runAgent(_ []string, stdout, _ io.Writer) int {
	fmt.Fprintf(stdout, stage0StubFmt,
		"agent",
		"agent",
		"Stage 9+ autopilot specs (cerebrate / overlord / drone)")
	return 0
}
