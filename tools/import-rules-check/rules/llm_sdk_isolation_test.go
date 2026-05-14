// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package rules

import (
	"strings"
	"testing"
)

func TestCheckLLMSDKIsolation_Allowed(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		from string
		to   string
	}{
		{
			name: "anthropic-leaf-imports-anthropic",
			from: "github.com/sqlrush/opendbx/internal/domain/llm/anthropic",
			to:   "github.com/anthropics/anthropic-sdk-go",
		},
		{
			name: "anthropic-leaf-subpkg-imports-sdk-sub",
			from: "github.com/sqlrush/opendbx/internal/domain/llm/anthropic/internal",
			to:   "github.com/anthropics/anthropic-sdk-go/option",
		},
		{
			name: "openai-leaf-imports-openai",
			from: "github.com/sqlrush/opendbx/internal/domain/llm/openai",
			to:   "github.com/openai/openai-go",
		},
		{
			name: "openai-leaf-imports-sashabaranov-fork",
			from: "github.com/sqlrush/opendbx/internal/domain/llm/openai",
			to:   "github.com/sashabaranov/go-openai",
		},
		{
			name: "non-llm-import",
			from: "github.com/sqlrush/opendbx/cmd/opendbx",
			to:   "github.com/sqlrush/opendbx/internal/platform/logger",
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := CheckLLMSDKIsolation(c.from, c.to); got != "" {
				t.Errorf("expected no violation; got %q", got)
			}
		})
	}
}

func TestCheckLLMSDKIsolation_Forbidden(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		from string
	}{
		{"cmd-direct", "github.com/sqlrush/opendbx/cmd/opendbx"},
		{"internal-app", "github.com/sqlrush/opendbx/internal/app/cli"},
		{"llm-parent-not-leaf", "github.com/sqlrush/opendbx/internal/domain/llm"},
		{"llm-model-sibling", "github.com/sqlrush/opendbx/internal/domain/llm/model"},
		{"tools-shouldnt-import-sdk", "github.com/sqlrush/opendbx/tools/some-tool"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := CheckLLMSDKIsolation(c.from, "github.com/anthropics/anthropic-sdk-go")
			if got == "" {
				t.Errorf("expected violation from %q; got empty", c.from)
			}
			if !strings.Contains(got, "IMP-7") {
				t.Errorf("expected IMP-7 marker; got %q", got)
			}
			if !strings.Contains(got, "规则 16") {
				t.Errorf("expected 规则 16 rationale; got %q", got)
			}
		})
	}
}
