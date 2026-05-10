// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Agent subcommand stub. Stage-9+ autopilot mode.

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newAgentCommand(_ *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "agent",
		Short: "Run as autopilot agent (Stage 9+)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(),
				"agent mode not yet implemented in spec-9.X (cerebrate / overlord / drone autopilot tiers).")
			return err
		},
	}
}
