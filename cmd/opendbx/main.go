// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package main is the opendbx binary entry point.
//
// spec-0.3 (cmd-entrypoints) replaces the spec-0.2 minimal flag-stdlib router
// with a cobra-based dispatch. main is intentionally tiny (≤ 50 LOC per
// spec § 1.1 D-10): it constructs the root command from cmd/opendbx/root.go,
// records a startup checkpoint via internal/platform/profileutil, and calls
// cobra.Execute. All real wiring lives in root.go / flags.go / *.go subcommand
// files.
//
// Per spec § 2.2 the only platform package main may import is
// internal/platform/version (the unique cmd → platform exception).
// Profile checkpoints route through internal/entrypoints.Checkpoint to
// preserve that invariant (spec-0.3 R2 fixup per codex H-6).
package main

import (
	"os"

	"github.com/sqlrush/opendbx/internal/entrypoints"
)

func main() {
	entrypoints.Checkpoint("main_entry")
	rootCmd := newRootCommand()
	if err := rootCmd.Execute(); err != nil {
		// cobra already prints the user-facing error; we just propagate exit.
		os.Exit(1)
	}
}
