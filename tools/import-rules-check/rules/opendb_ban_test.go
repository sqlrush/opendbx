// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package rules

import (
	"strings"
	"testing"
)

func TestCheckOpendbBan_Allowed(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		from string
		to   string
	}{
		{"opendbx-self", "github.com/sqlrush/opendbx/cmd/opendbx", "github.com/sqlrush/opendbx/internal/platform/version"},
		{"stdlib", "github.com/sqlrush/opendbx/internal/platform/logger", "fmt"},
		{"third-party-non-opendb", "github.com/sqlrush/opendbx/internal/platform/config", "go.yaml.in/yaml/v3"},
		{"opendb-substring-but-not-prefix", "github.com/sqlrush/opendbx/cmd", "github.com/example/opendb-utils"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := CheckOpendbBan(c.from, c.to); got != "" {
				t.Errorf("expected no violation; got %q", got)
			}
		})
	}
}

func TestCheckOpendbBan_Forbidden(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		to   string
	}{
		{"github-opendb-exact", "github.com/opendb"},
		{"github-opendb-root", "github.com/opendb/core"},
		{"github-opendb-deep", "github.com/opendb/core/internal/render"},
		{"github-opendb-project-exact", "github.com/opendb-project"},
		{"github-opendb-project", "github.com/opendb-project/something"},
		{"bitbucket-opendb", "bitbucket.org/opendb/legacy-render"},
		{"gitlab-opendb", "gitlab.com/opendb/fork"},
		{"gitee-opendb", "gitee.com/opendb/anything"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := CheckOpendbBan("github.com/sqlrush/opendbx/cmd", c.to)
			if got == "" {
				t.Errorf("expected violation for %q; got empty", c.to)
			}
			if !strings.Contains(got, "IMP-5") {
				t.Errorf("expected IMP-5 marker; got %q", got)
			}
			if !strings.Contains(got, "AD-001") {
				t.Errorf("expected AD-001 rationale; got %q", got)
			}
		})
	}
}

func TestCheckOpendbBan_FromIgnored(t *testing.T) {
	t.Parallel()
	// The rule only inspects `to`; the from is irrelevant. Any opendbx
	// caller importing opendb fails identically.
	if r1, r2 := CheckOpendbBan("a/b/c", "github.com/opendb/x"),
		CheckOpendbBan("z/y/x", "github.com/opendb/x"); r1 != r2 {
		t.Errorf("from must not affect verdict: %q vs %q", r1, r2)
	}
}
