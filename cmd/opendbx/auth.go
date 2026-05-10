// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Auth subcommand skeleton. Real implementation in Stage 2+ (LLM provider
// credential store; opendbx does NOT do Anthropic OAuth).

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newAuthCommand(_ *Options) *cobra.Command {
	auth := &cobra.Command{
		Use:   "auth",
		Short: "Manage authentication (LLM provider credentials)",
	}
	stub := func(use, short string) *cobra.Command {
		return &cobra.Command{
			Use:   use,
			Short: short,
			RunE: func(cmd *cobra.Command, _ []string) error {
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "auth %s not yet implemented in Stage 2+ (LLM provider credential vault).\n", use)
				return err
			},
		}
	}
	auth.AddCommand(
		stub("login", "Log in to an LLM provider"),
		stub("logout", "Log out / clear credentials"),
		stub("status", "Show current authentication status"),
	)
	return auth
}
