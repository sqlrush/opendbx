// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Completion subcommand. cobra has built-in completion generators; spec-0.7
// release flow may surface this as a real command. Currently it ships as a
// stub that points users at cobra's mechanism.

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newCompletionCommand(_ *Options) *cobra.Command {
	return &cobra.Command{
		Use:                   "completion <shell>",
		Short:                 "Generate shell completion script (bash/zsh/fish/powershell)",
		Args:                  cobra.ExactArgs(1),
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			shell := args[0]
			_, err := fmt.Fprintf(cmd.OutOrStdout(),
				"completion %s not yet activated in spec-0.7-release-flow (cobra mechanism is wired but disabled in stage-0).\n",
				shell)
			return err
		},
	}
}
