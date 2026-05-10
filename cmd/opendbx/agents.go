// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Agents subcommand stub. Lists configured autopilot agents (Stage 9+).

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newAgentsCommand(_ *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "agents",
		Short: "List configured agents (autopilot Stage 9+)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(),
				"agents listing not yet implemented in spec-9.X.")
			return err
		},
	}
}
