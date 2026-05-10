// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Sentinel subcommand skeleton (opendbx-specific, spec-0.3 D-4 NEW).
// Real probes land in Stage 1+ (spec-1.X-sentinel-* + spec-3.X full
// 48-metric Sentinel).

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newSentinelCommand(_ *Options) *cobra.Command {
	sentinel := &cobra.Command{
		Use:   "sentinel",
		Short: "DB metric probes (opendbx-specific, Stage 1+ for skeleton, Stage 3 for full)",
	}
	stub := func(use, short, spec string) *cobra.Command {
		return &cobra.Command{
			Use:   use,
			Short: short,
			RunE: func(cmd *cobra.Command, _ []string) error {
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "sentinel %s not yet implemented in %s.\n", use, spec)
				return err
			},
		}
	}
	sentinel.AddCommand(
		stub("start", "Start the sentinel probes", "Stage 1+"),
		stub("stop", "Stop the sentinel probes", "Stage 1+"),
		stub("status", "Show sentinel probe status", "Stage 1+"),
	)
	return sentinel
}
