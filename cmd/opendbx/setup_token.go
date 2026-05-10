// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Setup-token subcommand stub. opendbx LLM provider token store (Stage 2+).

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newSetupTokenCommand(_ *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "setup-token",
		Short: "Set up a long-lived LLM provider authentication token",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(),
				"setup-token not yet implemented in Stage 2+ (LLM provider token store).")
			return err
		},
	}
}
