// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// IMP-7 llm-sdk-isolation: forbid imports of LLM provider SDKs outside
// `internal/domain/llm/<provider>` leaf packages.
//
// Rationale (CLAUDE.md 规则 16):
//
// opendbx must remain model-agnostic — switching LLM provider should be a
// config change, not a code change. To enforce this, all direct
// dependencies on Anthropic / OpenAI / etc SDKs are scoped to dedicated
// `internal/domain/llm/<provider>` packages. The rest of opendbx talks to
// the `internal/domain/llm.Provider` interface only.
//
// This rule rejects any import of an LLM SDK from a package not under
// `internal/domain/llm/<provider>` leaf.

package rules

import (
	"fmt"
	"strings"
)

// LLMSDKPrefixes are the known LLM SDK module path prefixes. Add entries
// when adopting a new provider; the change must come with a spec or
// errata referencing the new SDK and its allowed home package.
// spec-0.10 D-3 IMP-7 / R2 codex MED-2: enumerate explicit list.
var LLMSDKPrefixes = []string{
	"github.com/anthropics/anthropic-sdk-go",
	"github.com/anthropics/anthropic-sdk-go/",
	"github.com/openai/openai-go",
	"github.com/openai/openai-go/",
	"github.com/sashabaranov/go-openai",
	"github.com/sashabaranov/go-openai/",
}

// LLMSDKAllowedPrefixes are the only opendbx import-path prefixes
// permitted to import LLM SDKs. spec-0.10 R2 codex LOW-5: leaf packages
// only, not the parent `internal/domain/llm` itself (which holds the
// model-agnostic Provider interface).
var LLMSDKAllowedPrefixes = []string{
	ModulePrefix + "internal/domain/llm/anthropic",
	ModulePrefix + "internal/domain/llm/openai",
}

// hasLLMSDKPrefix reports whether `to` matches any LLM SDK prefix.
func hasLLMSDKPrefix(to string) bool {
	for _, p := range LLMSDKPrefixes {
		// Match either exactly the bare package or any subpackage.
		if to == strings.TrimSuffix(p, "/") || strings.HasPrefix(to, p) {
			return true
		}
	}
	return false
}

// isLLMSDKAllowedSource reports whether `from` is permitted to import
// LLM SDKs (leaf-package check).
func isLLMSDKAllowedSource(from string) bool {
	for _, p := range LLMSDKAllowedPrefixes {
		if from == p || strings.HasPrefix(from, p+"/") {
			return true
		}
	}
	return false
}

// CheckLLMSDKIsolation returns "" if the from→to edge is allowed, or a
// violation describing the rule trip.
func CheckLLMSDKIsolation(from, to string) string {
	if !hasLLMSDKPrefix(to) {
		return ""
	}
	if isLLMSDKAllowedSource(from) {
		return ""
	}
	return fmt.Sprintf(
		"IMP-7 llm-sdk-isolation: %q imports LLM SDK %q; only internal/domain/llm/{anthropic,openai} may import LLM SDKs (CLAUDE.md 规则 16 model-agnostic)",
		from, to)
}
