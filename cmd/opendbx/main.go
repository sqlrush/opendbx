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
// spec-0.4 D-9: actual config.Load runs in root command's
// PersistentPreRunE (after cobra parses --settings + other flags). main()
// only sets up the root command + checkpoints.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/sqlrush/opendbx/internal/entrypoints"
	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

func main() {
	entrypoints.Checkpoint("main_entry")

	// spec-0.5 claude HIGH-2 + CC parity: pre-process argv so `-d2e` is
	// expanded to `--debug-to-stderr` before cobra sees it. Without this,
	// pflag parses `-d2e` as `-d=2e` (because `-d` is the shorthand for
	// `--debug` registered as a string flag) — sets Debug filter to "2e"
	// and never toggles DebugToStderr. CC does the equivalent expansion in
	// main.tsx before commander.parse.
	os.Args = preprocessShortFlags(os.Args)

	rootCmd := newRootCommand()

	// spec-0.5 codex CRIT-1: ensure logger flushes on every exit path —
	// normal, error, panic. RegisterSignalCleanup (called in PersistentPreRunE)
	// only covers SIGINT/SIGTERM; we still need an explicit Close before
	// os.Exit and a GuardPanic around Execute so buffered JSONL events
	// don't get dropped on the common exit paths.
	var exitCode int
	entrypoints.GuardLoggerPanic(func() {
		if err := rootCmd.Execute(); err != nil {
			// cobra already printed `Error: [CODE] message`. T-13 codex M-7:
			// surface errcode Hint() on a follow-on line so users have
			// actionable guidance (spec § 2.4 contract: Code/Message/Hint).
			var ecErr errcode.Error
			if errors.As(err, &ecErr) && ecErr.Hint() != "" {
				_, _ = fmt.Fprintf(os.Stderr, "  hint: %s\n", ecErr.Hint())
			}
			exitCode = 1
		}
	})
	_ = entrypoints.CloseLogger()
	if exitCode != 0 {
		os.Exit(exitCode)
	}
}

// preprocessShortFlags rewrites compact-form shortflags that pflag would
// otherwise mis-parse. Currently handles:
//
//	-d2e   →   --debug-to-stderr
//
// (CC parity — `-d2e` is documented as the short form of `--debug-to-stderr`.
// Because `-d` is already the registered shorthand for `--debug` (a string
// flag), pflag's default tokenisation reads `-d2e` as `-d=2e` and sets the
// debug FILTER to "2e" rather than enabling stderr output. The rewrite
// happens once at program entry; argv ordering is preserved.)
func preprocessShortFlags(args []string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		switch a {
		case "-d2e":
			out = append(out, "--debug-to-stderr")
		default:
			out = append(out, a)
		}
	}
	return out
}
