// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// MCP subcommand skeleton (spec-0.3 D-4).
// Mirrors CC main.tsx L3894-3956 mcp command tree. Real implementation in
// spec-2.5 ~ spec-2.7.

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newMCPCommand(_ *Options) *cobra.Command {
	mcp := &cobra.Command{
		Use:   "mcp",
		Short: "Configure and manage MCP servers",
	}

	stub := func(use, short, spec string) *cobra.Command {
		return &cobra.Command{
			Use:   use,
			Short: short,
			RunE: func(cmd *cobra.Command, _ []string) error {
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "mcp %s not yet implemented in %s.\n", use, spec)
				return err
			},
		}
	}

	mcp.AddCommand(
		stub("serve", "Start the opendbx MCP server", "spec-2.5"),
		stub("add <name>", "Add an MCP server", "spec-2.5"),
		stub("remove <name>", "Remove an MCP server", "spec-2.5"),
		stub("list", "List configured MCP servers", "spec-2.5"),
		stub("get <name>", "Get details about an MCP server", "spec-2.5"),
		stub("add-json <name> <json>", "Add an MCP server (stdio or SSE) with a JSON string", "spec-2.5"),
		stub("add-from-claude-desktop", "Import MCP servers from Claude Desktop (Mac and WSL only)", "spec-2.6"),
		stub("reset-project-choices", "Reset all approved/rejected project-scoped (.mcp.json) servers", "spec-2.5"),
	)
	return mcp
}
