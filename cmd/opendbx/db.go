// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// DB subcommand skeleton (opendbx-specific, spec-0.3 D-4 NEW).
// Real connection management lands in spec-1.18-pg-driver +
// spec-1.19-connection-config.

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newDBCommand(_ *Options) *cobra.Command {
	db := &cobra.Command{
		Use:   "db",
		Short: "Manage database connection aliases (opendbx-specific)",
	}
	stub := func(use, short, spec string) *cobra.Command {
		return &cobra.Command{
			Use:   use,
			Short: short,
			RunE: func(cmd *cobra.Command, _ []string) error {
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "db %s not yet implemented in %s.\n", use, spec)
				return err
			},
		}
	}
	db.AddCommand(
		stub("add <alias> <dsn>", "Save a connection alias", "spec-1.19-connection-config"),
		stub("remove <alias>", "Remove a connection alias", "spec-1.19-connection-config"),
		stub("list", "List saved connection aliases", "spec-1.19-connection-config"),
		stub("test <alias>", "Test a saved connection", "spec-1.18-pg-driver"),
	)
	return db
}
