// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Interact subcommand (spec-0.3 D-4 + D-10).
// Stage-0 stub; real REPL lands in spec-1.16-input-three-modes.
//
// Behaviour parity: `opendbx` (no subcommand) and `opendbx interact` produce
// identical output. Both go through runInteractRoot in root.go.

package main

import "github.com/spf13/cobra"

func newInteractCommand(opts *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "interact [prompt]",
		Short: "Start an interactive diagnosis session (default mode)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Session.Prompt = args[0]
			}
			return runInteractRoot(cmd, opts)
		},
	}
}
