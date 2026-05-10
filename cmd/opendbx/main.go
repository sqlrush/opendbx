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
// internal/platform/version. Config + profile checkpoints route through
// internal/entrypoints to preserve that invariant.
//
// spec-0.4 D-9: cobra.Execute is preceded by config.Load (via entrypoints
// relay). Load failure exits 1 with the validation error written to stderr
// — fail-fast per spec § 3.1.
package main

import (
	"fmt"
	"os"

	"github.com/sqlrush/opendbx/internal/entrypoints"
)

func main() {
	entrypoints.Checkpoint("main_entry")

	// spec-0.4 D-9: load config before cobra dispatches, so subcommands can
	// observe the resolved tree via entrypoints.LoadConfigDefault. Failure
	// here is reported and exits 1; cobra never gets called.
	entrypoints.Checkpoint("config_load_start")
	if _, err := entrypoints.LoadConfigDefault(); err != nil {
		fmt.Fprintf(os.Stderr, "opendbx: config load failed:\n%v\n", err)
		os.Exit(1)
	}
	entrypoints.Checkpoint("config_loaded")

	rootCmd := newRootCommand()
	if err := rootCmd.Execute(); err != nil {
		// cobra already prints the user-facing error; we just propagate exit.
		os.Exit(1)
	}
}
