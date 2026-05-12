// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// `opendbx version` subcommand — explicit form of `opendbx --version`.
// Output format (per user D7): "<version> (opendbx)\n" matching CC's
// "${VERSION} (Claude Code)" shape.
//
// spec-0.7 T-6: when invoked as `opendbx version --version-verbose`, the
// subcommand emits the Verbose() multi-line diagnostic block — same output
// as `opendbx --version-verbose`. Because the root --version-verbose is
// declared as a non-persistent Flag (spec D-4: scope mirrors --version),
// the subcommand registers its own independent --version-verbose flag for
// the `opendbx version --version-verbose` invocation path.

package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sqlrush/opendbx/internal/platform/version"
)

func newVersionCommand(_ *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version and exit",
		RunE: func(cmd *cobra.Command, _ []string) error {
			// First check own flag (set when user typed
			// `opendbx version --version-verbose`); fall back to root flag
			// (set when user typed `opendbx --version-verbose version`).
			if vv, _ := cmd.Flags().GetBool("version-verbose"); vv {
				_, err := fmt.Fprint(cmd.OutOrStdout(), version.Verbose())
				return err
			}
			if vv, _ := cmd.Root().Flags().GetBool("version-verbose"); vv {
				_, err := fmt.Fprint(cmd.OutOrStdout(), version.Verbose())
				return err
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s (opendbx)\n", version.String())
			return err
		},
	}
	cmd.Flags().Bool("version-verbose", false,
		"Output build version + commit + build date + Go runtime + os/arch (multi-line)")
	return cmd
}
