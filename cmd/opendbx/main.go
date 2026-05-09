// Copyright 2026 opendbx contributors. See LICENSE.
//
// Package main is the opendbx binary entry point.
//
// Stage 0 minimal router: parses os.Args[1] and dispatches to subcommand
// stubs (interact / agent / cluster / admin). Subcommand bodies print
// "not yet implemented in stage 0" until the relevant spec lands.
//
// Design: opendbrb/specs/stage-0/spec-0.2-go-module-layout.md § 2.1, D-2 + D-3.
// Author: sqlrush
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/sqlrush/opendbx/internal/platform/version"
)

// helpText is printed for `opendbx --help` / `opendbx help` / no-args.
//
// CC-aligned structure (CC: `claude --help` shows name + description +
// subcommand list; spec-0.3 will land 1:1 alignment of wording with
// ~/claude-code-source-code/src/main.tsx). spec-0.2 ships the structural
// skeleton; spec-0.3 backlog tracks per-line CC diff.
const helpText = `opendbx — DB-focused Claude-Code-style agent platform.

USAGE:
  opendbx [SUBCOMMAND] [flags]

SUBCOMMANDS:
  interact       Start interactive TUI session (default)
  agent          Run as autopilot agent (Stage 9+)
  cluster        Cluster mode commands (Stage 9+)
  admin          Administrative commands (migrations, etc.)
  help           Show this help message
  version        Print version and exit

FLAGS:
  -h, --help     Show help
  -v, --version  Show version

Run 'opendbx <subcommand> --help' for more on a subcommand.

See https://github.com/sqlrush/opendbrb (private) for design docs.
`

// stage0StubFmt is the format string emitted by every subcommand stub.
// Args: subcommand name, subcommand name, target spec id.
const stage0StubFmt = `opendbx %s — not yet implemented in stage 0.

Stage 0 is the bootstrap phase. The %s subcommand will land in:

  %s

Track progress at https://github.com/sqlrush/opendbrb (private).
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is the testable entrypoint. Decoupled from os.Stdout/Stderr so
// CLI golden tests can capture output via bytes.Buffer.
//
// Returns the process exit code.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stdout, helpText)
		return 0
	}

	switch args[0] {
	case "-v", "--version", "version":
		fmt.Fprintf(stdout, "opendbx %s\n", version.String())
		return 0

	case "-h", "--help", "help":
		fmt.Fprint(stdout, helpText)
		return 0

	case "interact":
		return runInteract(args[1:], stdout, stderr)

	case "agent":
		return runAgent(args[1:], stdout, stderr)

	case "cluster":
		return runCluster(args[1:], stdout, stderr)

	case "admin":
		return runAdmin(args[1:], stdout, stderr)

	default:
		fmt.Fprintf(stderr, "Unknown subcommand: %q\n\n", args[0])
		fmt.Fprint(stderr, helpText)
		return 1
	}
}
