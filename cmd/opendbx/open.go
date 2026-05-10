// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Open subcommand stub. Connect to a remote opendbx server (spec-2.6+).
// CC equivalent: `claude open <cc-url>` (main.tsx L4059).

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newOpenCommand(_ *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "open <opendbx-url>",
		Short: "Connect to a remote opendbx server (use opendbx:// URLs)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(),
				"open %s not yet implemented in spec-2.6+ (remote session protocol).\n", args[0])
			return err
		},
	}
}
