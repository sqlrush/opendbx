// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Root cobra command construction (spec-0.3 D-2 + D-10).
//
// Mirrors CC main.tsx L968:
//
//	program.name('claude')
//	  .description('Claude Code - starts an interactive session by default, use -p/--print for non-interactive output')
//	  .argument('[prompt]', 'Your prompt', String)
//	  .helpOption('-h, --help', 'Display help for command')
//
// Adaptation rationale (per spec-0.3 § 1.4 + § 2.3): replace "Claude Code" /
// "codebase" wording with DB-focused phrasing while keeping argument and flag
// shapes 1:1. See opendbrb/docs/cc-vs-opendbx-help-diff.md for line-by-line
// rationale.

package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sqlrush/opendbx/internal/platform/version"
)

// newRootCommand constructs the opendbx root cobra command.
//
// Behavior on no subcommand (user D1 decision, spec-0.3 § 3.1):
//   - opendbx           → interact stub (matches CC `claude` REPL launch)
//   - opendbx <prompt>  → interact stub seeded with prompt (matches CC L968 [prompt] positional)
//   - opendbx -v / --version → "<version> (opendbx)\n" (user D7, matches CC `${VERSION} (Claude Code)` shape)
//   - opendbx -h / --help / opendbx help → cobra-rendered help
func newRootCommand() *cobra.Command {
	opts := newOptions()

	rootCmd := &cobra.Command{
		Use:   "opendbx [prompt]",
		Short: "DB-focused Claude-Code-style agent platform",
		Long:  "opendbx - starts an interactive session by default, use -p/--print for non-interactive output",
		// no-args + [prompt] both go through RunE → interact stub
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true, // suppress "Usage: ..." dump on RunE-returned errors
		SilenceErrors: false,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Manual --version handling (cobra's built-in only supports --version,
			// no -v shorthand; we emulate CC commander's `-v, --version` shape).
			if v, _ := cmd.Flags().GetBool("version"); v {
				fmt.Fprintf(cmd.OutOrStdout(), "%s (opendbx)\n", version.String())
				return nil
			}
			if len(args) > 0 {
				opts.Session.Prompt = args[0]
			}
			return runInteractRoot(cmd, opts)
		},
	}

	// Top-level flags including -v/--version (see flags.go).
	registerFlags(rootCmd, opts)

	// Subcommands (see individual *.go files).
	rootCmd.AddCommand(
		newInteractCommand(opts),
		newAgentCommand(opts),
		newClusterCommand(opts),
		newAdminCommand(opts),
		newMCPCommand(opts),
		newPluginCommand(opts),
		newAuthCommand(opts),
		newAgentsCommand(opts),
		newDoctorCommand(opts),
		newUpdateCommand(opts),
		newInstallCommand(opts),
		newSetupTokenCommand(opts),
		newCompletionCommand(opts),
		newOpenCommand(opts),
		newDBCommand(opts),
		newSentinelCommand(opts),
		newDiagCommand(opts),
		newVersionCommand(opts),
	)

	return rootCmd
}

// runInteractRoot is shared between the root command (no subcommand) and the
// explicit `opendbx interact` subcommand (interact.go). spec-0.3 returns a
// stage-0 stub; spec-1.16-input-three-modes wires up the real REPL.
func runInteractRoot(cmd *cobra.Command, opts *Options) error {
	if opts.Session.Prompt != "" {
		fmt.Fprintf(cmd.OutOrStdout(),
			"interact mode not yet implemented (spec-1.16); received prompt: %q\n",
			opts.Session.Prompt)
	} else {
		fmt.Fprintln(cmd.OutOrStdout(),
			"interact mode not yet implemented (spec-1.16). Run with -h for help.")
	}
	return nil
}
