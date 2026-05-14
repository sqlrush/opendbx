// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package rules

import (
	"strings"
	"testing"
)

func TestCheckRunewidthWrap_Allowed(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		from string
		to   string
	}{
		{
			name: "wrapper-root-imports-runewidth",
			from: "github.com/sqlrush/opendbx/internal/app/cli/render/width",
			to:   "github.com/mattn/go-runewidth",
		},
		{
			name: "wrapper-subpkg-imports-runewidth",
			from: "github.com/sqlrush/opendbx/internal/app/cli/render/width/internal",
			to:   "github.com/mattn/go-runewidth",
		},
		{
			name: "non-runewidth-import",
			from: "github.com/sqlrush/opendbx/cmd/opendbx",
			to:   "github.com/sqlrush/opendbx/internal/platform/logger",
		},
		{
			name: "lookalike-package-passes",
			from: "github.com/sqlrush/opendbx/cmd",
			to:   "github.com/mattn/go-runewidth-fork",
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := CheckRunewidthWrap(c.from, c.to); got != "" {
				t.Errorf("expected no violation; got %q", got)
			}
		})
	}
}

func TestCheckRunewidthWrap_Forbidden(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		from string
	}{
		{"cmd-direct", "github.com/sqlrush/opendbx/cmd/opendbx"},
		{"render-sibling-block", "github.com/sqlrush/opendbx/internal/app/cli/render/block"},
		{"render-sibling-layout", "github.com/sqlrush/opendbx/internal/app/cli/render/layout"},
		{"width-substring-not-prefix", "github.com/sqlrush/opendbx/internal/app/cli/render/widthly"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := CheckRunewidthWrap(c.from, "github.com/mattn/go-runewidth")
			if got == "" {
				t.Errorf("expected violation from %q; got empty", c.from)
			}
			if !strings.Contains(got, "IMP-8") {
				t.Errorf("expected IMP-8 marker; got %q", got)
			}
			if !strings.Contains(got, "render/width") {
				t.Errorf("expected wrapper rationale; got %q", got)
			}
		})
	}
}
