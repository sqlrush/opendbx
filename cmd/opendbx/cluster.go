// Copyright 2026 opendbx contributors. See LICENSE.
//
// Stage 0 stub: cluster mode. Lands in Stage 9+.
//
// Design: opendbrb/specs/stage-0/spec-0.2-go-module-layout.md D-2.
// Author: sqlrush
package main

import (
	"fmt"
	"io"
)

func runCluster(_ []string, stdout, _ io.Writer) int {
	fmt.Fprintf(stdout, stage0StubFmt,
		"cluster",
		"cluster",
		"Stage 9+ cluster specs")
	return 0
}
