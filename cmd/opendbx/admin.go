// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Admin subcommand stub. Migrations / config maintenance lands in
// spec-4.8-version-migrations.
//
// Per spec-0.3 § 2.2 (carried from spec-0.2 § 2.2 重要细则 #1):
// `cmd/opendbx/admin.go` does NOT import internal/platform/migrations
// directly. When real `admin migrate` lands, it dispatches to
// internal/entrypoints/admin → internal/bootstrap → migrations.

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newAdminCommand(_ *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "admin",
		Short: "Administrative commands (migrations, config maintenance)",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "migrate",
		Short: "Run pending database/state migrations",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(),
				"admin migrate not yet implemented in spec-4.8-version-migrations.")
			return err
		},
	})
	return cmd
}
