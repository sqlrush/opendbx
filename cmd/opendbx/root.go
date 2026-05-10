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
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sqlrush/opendbx/internal/entrypoints"
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
		// CC parity: `opendbx hello world` joins args as a single prompt,
		// matching `claude hello world`. Arbitrary args allowed; bare
		// non-subcommand tokens become [prompt].
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true, // suppress "Usage: ..." dump on RunE-returned errors
		SilenceErrors: false,
		// PersistentPreRunE runs BEFORE the subcommand's RunE and inherits to
		// every subcommand without a *PreRunE of its own. We use it to:
		//   1. Build LoadOptions from the parsed Options (--settings,
		//      --output-format, --model, ...) and run config.Load.
		//   2. Stash the resolved *Config in the cobra context for any
		//      subcommand that wants it (admin config sources, etc.).
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			entrypoints.Checkpoint("config_load_start")
			cfg, err := entrypoints.LoadConfigFromCLI(buildCLIInputs(opts))
			if err != nil {
				return err
			}
			entrypoints.Checkpoint("config_loaded")
			cmd.SetContext(entrypoints.WithConfig(cmd.Context(), cfg))
			return validateChoiceFlags(cmd, opts)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Manual --version handling (cobra's built-in only supports --version,
			// no -v shorthand; we emulate CC commander's `-v, --version` shape).
			if v, _ := cmd.Flags().GetBool("version"); v {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s (opendbx)\n", version.String())
				return nil
			}
			if len(args) > 0 {
				opts.Session.Prompt = strings.Join(args, " ")
			}
			return runInteractRoot(cmd, opts)
		},
		// spec § 7 DoD D-9: --debug=profile triggers profile report on stderr.
		// Hotfix (spec-0.3): moved from RunE to PersistentPostRunE so subcommand
		// paths (mcp/db/admin/...) also surface profile output. Per cobra docs,
		// PersistentPostRunE inherited by subcommands when their own *PostRunE
		// is unset (which is the case for all stage-0 stubs).
		PersistentPostRunE: func(cmd *cobra.Command, _ []string) error {
			if strings.Contains(opts.Debug.Debug, "profile") {
				entrypoints.ReportProfile(cmd.ErrOrStderr())
			}
			return nil
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

// buildCLIInputs constructs the entrypoints.CLILoadInputs from the parsed
// cobra Options. Implements the spec § 1.1 D-2 override chain top
// (FlagSettingsPath + FlagOverrides). Every flag the user actually set
// becomes a SourceCLIFlag override.
//
// cmd/opendbx imports only entrypoints (not config) to preserve the
// cmd → platform/version single exception per spec-0.3 hotfix.
func buildCLIInputs(opts *Options) entrypoints.CLILoadInputs {
	cwd, _ := os.Getwd()
	in := entrypoints.CLILoadInputs{
		CWD:          cwd,
		SettingsPath: opts.IO.Settings,
	}
	if opts.Print.OutputFormat != "" {
		in.Overrides = append(in.Overrides,
			entrypoints.FlagOverride{Path: "Output.Format", Value: opts.Print.OutputFormat})
	}
	if opts.Model.Model != "" {
		in.Overrides = append(in.Overrides,
			entrypoints.FlagOverride{Path: "LLM.ActiveModel", Value: opts.Model.Model})
	}
	if opts.Model.LLMTier != "" {
		in.Overrides = append(in.Overrides,
			entrypoints.FlagOverride{Path: "LLM.Tier", Value: opts.Model.LLMTier})
	}
	return in
}

// runInteractRoot is shared between the root command (no subcommand) and the
// explicit `opendbx interact` subcommand (interact.go). spec-0.3 returns a
// stage-0 stub; spec-1.16-input-three-modes wires up the real REPL.
func runInteractRoot(cmd *cobra.Command, opts *Options) error {
	if opts.Session.Prompt != "" {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(),
			"interact mode not yet implemented (spec-1.16); received prompt: %q\n",
			opts.Session.Prompt)
	} else {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(),
			"interact mode not yet implemented (spec-1.16). Run with -h for help.")
	}
	return nil
}

// validateChoiceFlags enforces enum-typed flags (spec § 3.1: invalid choice
// → exit 1). cobra/pflag has no native choice constraint for vanilla string
// vars (cobra v1.x's `RegisterFlagCompletionFunc` is for completion, not
// validation), so we check after-the-fact in PreRunE.
func validateChoiceFlags(_ *cobra.Command, opts *Options) error {
	if opts.Print.OutputFormat != "" && !contains(validOutputFormats, opts.Print.OutputFormat) {
		return fmt.Errorf("invalid --output-format %q (must be one of: %s)",
			opts.Print.OutputFormat, strings.Join(validOutputFormats, ", "))
	}
	if opts.Print.InputFormat != "" && !contains(validInputFormats, opts.Print.InputFormat) {
		return fmt.Errorf("invalid --input-format %q (must be one of: %s)",
			opts.Print.InputFormat, strings.Join(validInputFormats, ", "))
	}
	if opts.IO.PermissionMode != "" && !contains(validPermissionModes, opts.IO.PermissionMode) {
		return fmt.Errorf("invalid --permission-mode %q (must be one of: %s)",
			opts.IO.PermissionMode, strings.Join(validPermissionModes, ", "))
	}
	if opts.Print.MaxBudgetUSD < 0 {
		return fmt.Errorf("--max-budget-usd must be ≥ 0 (got %.2f)", opts.Print.MaxBudgetUSD)
	}
	return nil
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}
