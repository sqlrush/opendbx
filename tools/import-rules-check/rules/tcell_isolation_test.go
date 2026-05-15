// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package rules

import (
	"strings"
	"testing"
)

func TestCheckTcellIsolation_Allowed(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		from string
		to   string
	}{
		{
			name: "terminal-imports-tcell",
			from: "github.com/sqlrush/opendbx/internal/platform/terminal",
			to:   "github.com/gdamore/tcell/v2",
		},
		{
			name: "tui-imports-tcell",
			from: "github.com/sqlrush/opendbx/internal/app/cli/tui",
			to:   "github.com/gdamore/tcell/v2",
		},
		{
			name: "tui-subpackage-imports-tcell-subpkg",
			from: "github.com/sqlrush/opendbx/internal/app/cli/tui/internal",
			to:   "github.com/gdamore/tcell/v2/encoding",
		},
		{
			name: "bootstrap-imports-tcell",
			from: "github.com/sqlrush/opendbx/internal/bootstrap",
			to:   "github.com/gdamore/tcell/v2",
		},
		{
			name: "non-tcell-import",
			from: "github.com/sqlrush/opendbx/cmd/opendbx",
			to:   "github.com/sqlrush/opendbx/internal/app/cli/tui",
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := CheckTcellIsolation(c.from, c.to); got != "" {
				t.Errorf("expected no violation; got %q", got)
			}
		})
	}
}

func TestCheckTcellIsolation_Forbidden(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		from string
	}{
		{"cmd-direct", "github.com/sqlrush/opendbx/cmd/opendbx"},
		{"internal-app-cli-render", "github.com/sqlrush/opendbx/internal/app/cli/render"},
		{"internal-domain", "github.com/sqlrush/opendbx/internal/domain/db"},
		{"tools-shouldnt-import-tcell", "github.com/sqlrush/opendbx/tools/some-tool"},
		{"lookalike-not-allowed-prefix", "github.com/sqlrush/opendbx/internal/platform/terminalish"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := CheckTcellIsolation(c.from, "github.com/gdamore/tcell/v2")
			if got == "" {
				t.Errorf("expected violation from %q; got empty", c.from)
			}
			if !strings.Contains(got, "IMP-9") {
				t.Errorf("expected IMP-9 marker; got %q", got)
			}
			if !strings.Contains(got, "AD-002") {
				t.Errorf("expected AD-002 rationale; got %q", got)
			}
		})
	}
}

// boundary-safe: a lookalike module path containing tcell should not be
// flagged as a tcell import (e.g. "github.com/example/tcell-mock-v2").
func TestCheckTcellIsolation_BoundarySafe(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		to   string
	}{
		{"lookalike-prefix-distinct", "github.com/gdamore/tcell-extras"},
		{"unrelated-package", "github.com/some/tcell-mock"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := CheckTcellIsolation("github.com/sqlrush/opendbx/cmd/opendbx", c.to); got != "" {
				t.Errorf("expected no violation for lookalike %q; got %q", c.to, got)
			}
		})
	}
}
