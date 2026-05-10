// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// `opendbx version` subcommand — explicit form of `opendbx --version`.
// Output format (per user D7): "<version> (opendbx)\n" matching CC's
// "${VERSION} (Claude Code)" shape.

package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sqlrush/opendbx/internal/platform/version"
)

func newVersionCommand(_ *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version and exit",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s (opendbx)\n", version.String())
			return err
		},
	}
}
