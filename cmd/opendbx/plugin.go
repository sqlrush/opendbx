// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Plugin (alias plugins) subcommand. `plugin list` runs a one-shot skill
// discovery (spec-2.2); add/remove remain stubs until the full plugin tree
// (spec-2.18).

package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sqlrush/opendbx/internal/entrypoints"
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
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "plugin %s not yet implemented (spec-2.18).\n", use)
				return err
			},
		}
	}
	list := &cobra.Command{
		Use:   "list",
		Short: "List discovered skills (active / shadowed / conflicts / errors)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			summary, err := entrypoints.SkillsSummary()
			if err != nil {
				return err
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), summary)
			return err
		},
	}
	plugin.AddCommand(
		stub("add <name>", "Add a plugin"),
		stub("remove <name>", "Remove a plugin"),
		list,
	)
	return plugin
}
