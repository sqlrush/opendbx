// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Doctor subcommand stub.

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newDoctorCommand(_ *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check the health of opendbx (config + connectivity + auto-updater)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(),
				"doctor not yet implemented in Stage 4+ (config + connectivity + auto-updater health).")
			return err
		},
	}
}
