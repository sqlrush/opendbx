// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Admin subcommand. spec-0.4 lands `admin config` 5 verbs; `admin migrate`
// stays stub until spec-4.8.
//
// Per spec-0.3 § 2.2 (carried from spec-0.2 § 2.2 重要细则 #1): cmd/opendbx
// does NOT import internal/platform/* directly (except platform/version).
// All admin logic dispatches to internal/entrypoints/* relays.

package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sqlrush/opendbx/internal/entrypoints"
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
	cmd.AddCommand(newAdminConfigCommand())
	return cmd
}

// newAdminConfigCommand builds `opendbx admin config <verb>` (spec-0.4 D-8).
//
// Verbs (5 total per user R3 Q7):
//   - validate <file>    parse + validate one yaml file
//   - dump-defaults      Default() Config as YAML (with redaction)
//   - dump-schema        JSON Schema for Config
//   - sources [field]    per-field source provenance
//   - dump-env-map       OPENDBX_* → Section.Field mapping
func newAdminConfigCommand() *cobra.Command {
	cfgCmd := &cobra.Command{
		Use:   "config",
		Short: "Manage opendbx configuration",
	}

	cfgCmd.AddCommand(&cobra.Command{
		Use:   "validate <file>",
		Short: "Validate a config yaml file (no merging; isolated check)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := entrypoints.ValidateFile(args[0]); err != nil {
				return err
			}
			_, err := fmt.Fprintln(cmd.OutOrStdout(), "config OK")
			return err
		},
	})

	cfgCmd.AddCommand(&cobra.Command{
		Use:   "dump-defaults",
		Short: "Print the default config as YAML (secrets redacted)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return entrypoints.DumpDefaults(cmd.OutOrStdout())
		},
	})

	cfgCmd.AddCommand(&cobra.Command{
		Use:   "dump-schema",
		Short: "Print the JSON Schema describing the config tree",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return entrypoints.DumpSchema(cmd.OutOrStdout())
		},
	})

	cfgCmd.AddCommand(&cobra.Command{
		Use:   "sources [field]",
		Short: "Show the source (default/user/project/local/flag-settings/env/cli) of each config field, or just <field>",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := entrypoints.LoadConfigDefault()
			if err != nil {
				return err
			}
			field := ""
			if len(args) == 1 {
				field = args[0]
			}
			return entrypoints.DescribeSources(cmd.OutOrStdout(), cfg, field)
		},
	})

	cfgCmd.AddCommand(&cobra.Command{
		Use:   "dump-env-map",
		Short: "Print the OPENDBX_* environment variable → config-field mapping",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return entrypoints.DumpEnvMap(cmd.OutOrStdout())
		},
	})

	return cfgCmd
}
