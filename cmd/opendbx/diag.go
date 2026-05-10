// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Diag subcommand skeleton (opendbx-specific, spec-0.3 D-4 NEW).
// Real one-shot diagnoses land in spec-1.21-diagnose-loop and Stage 3 sentinel.

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newDiagCommand(_ *Options) *cobra.Command {
	diag := &cobra.Command{
		Use:   "diag",
		Short: "Run one-shot DB diagnoses (opendbx-specific)",
	}
	stub := func(use, short, spec string) *cobra.Command {
		return &cobra.Command{
			Use:   use,
			Short: short,
			RunE: func(cmd *cobra.Command, _ []string) error {
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "diag %s not yet implemented in %s.\n", use, spec)
				return err
			},
		}
	}
	diag.AddCommand(
		stub("slow-sql", "Diagnose slow SQL", "spec-1.21-diagnose-loop"),
		stub("lock-wait", "Diagnose lock waits", "spec-1.21-diagnose-loop"),
	)
	return diag
}
