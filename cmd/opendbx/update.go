// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Update / upgrade subcommand stub.

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newUpdateCommand(_ *Options) *cobra.Command {
	return &cobra.Command{
		Use:     "update",
		Aliases: []string{"upgrade"},
		Short:   "Check for updates and install if available",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(),
				"update not yet implemented in spec-4.7-install (auto-update infrastructure).")
			return err
		},
	}
}
