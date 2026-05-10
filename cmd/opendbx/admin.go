// Copyright 2026 opendbx contributors. See LICENSE.
//
// Stage 0 stub: administrative commands (migrations etc.).
//
// Per spec-0.2 § 2.2 重要细则 #1, this file does **not** import
// internal/platform/migrations directly. When the real `admin migrate`
// lands (spec-4.8), this stub will dispatch to internal/entrypoints/admin,
// which in turn calls internal/bootstrap to drive the migration flow.
// cmd never imports migrations directly — the unique cmd → platform
// exception is platform/version only.
//
// Design: opendbrb/specs/stage-0/spec-0.2-go-module-layout.md D-2.
// Author: sqlrush
package main

import (
	"fmt"
	"io"
)

func runAdmin(_ []string, stdout, _ io.Writer) int {
	_, _ = fmt.Fprintf(stdout, stage0StubFmt,
		"admin",
		"admin",
		"spec-4.8-version-migrations (admin migrate) + Stage 4+ admin specs")
	return 0
}
