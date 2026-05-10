// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Install subcommand stub. Native build installer lands in spec-4.7-install.

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newInstallCommand(_ *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "install [target]",
		Short: "Install opendbx native build. Use [target] to specify version (stable, latest, or specific version)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := "stable"
			if len(args) > 0 {
				target = args[0]
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(),
				"install (target=%s) not yet implemented in spec-4.7-install.\n", target)
			return err
		},
	}
}
