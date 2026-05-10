// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Plugin (alias plugins) subcommand skeleton. Real implementation in
// spec-2.1-skill-md-format.

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newPluginCommand(_ *Options) *cobra.Command {
	plugin := &cobra.Command{
		Use:     "plugin",
		Aliases: []string{"plugins"},
		Short:   "Manage opendbx plugins (Skills)",
	}
	stub := func(use, short string) *cobra.Command {
		return &cobra.Command{
			Use:   use,
			Short: short,
			RunE: func(cmd *cobra.Command, _ []string) error {
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "plugin %s not yet implemented in spec-2.1-skill-md-format.\n", use)
				return err
			},
		}
	}
	plugin.AddCommand(
		stub("add <name>", "Add a plugin"),
		stub("remove <name>", "Remove a plugin"),
		stub("list", "List installed plugins"),
	)
	return plugin
}
