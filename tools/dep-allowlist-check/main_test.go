// Copyright 2026 opendbx contributors. See LICENSE.
//
// Tests for dep-allowlist-check.
//
// Author: sqlrush
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAllowlist_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "allowlist.yml")
	body := `
direct_allowed:
  - module: github.com/foo/bar
    purpose: example
    introduced_by: spec-1.0
transitive_lock:
  - module: golang.org/x/text
    version: v0.14.0
tool_only:
  - module: golang.org/x/tools
    purpose: go/packages
    introduced_by: spec-0.2
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := loadAllowlist(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(a.DirectAllowed) != 1 || a.DirectAllowed[0].Module != "github.com/foo/bar" {
		t.Errorf("direct mismatch: %+v", a.DirectAllowed)
	}
	if len(a.TransitiveLock) != 1 || a.TransitiveLock[0].Version != "v0.14.0" {
		t.Errorf("transitive mismatch: %+v", a.TransitiveLock)
	}
	if len(a.ToolOnly) != 1 || a.ToolOnly[0].Module != "golang.org/x/tools" {
		t.Errorf("tool_only mismatch: %+v", a.ToolOnly)
	}
}

func TestLoadAllowlist_MissingIntroducedBy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "allowlist.yml")
	body := `
direct_allowed:
  - module: github.com/foo/bar
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadAllowlist(path); err == nil {
		t.Error("expected error on missing introduced_by")
	}
}

func TestLoadAllowlist_MissingTransitiveVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "allowlist.yml")
	body := `
transitive_lock:
  - module: golang.org/x/text
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadAllowlist(path); err == nil {
		t.Error("expected error on missing version")
	}
}

func TestViolatesToolOnly(t *testing.T) {
	toolOnly := map[string]struct{}{
		"golang.org/x/tools": {},
		"gopkg.in/yaml.v3":   {},
	}
	cases := []struct {
		path string
		want bool
	}{
		{"golang.org/x/tools", true},
		{"golang.org/x/tools/go/packages", true},
		{"golang.org/x/tools/go", true},
		{"gopkg.in/yaml.v3", true},
		{"gopkg.in/yaml.v3/internal", true},
		{"github.com/foo/bar", false},
		{"golang.org/x/text", false},    // not in set
		{"golang.org/x/toolset", false}, // prefix-but-not-slash
		{"github.com/sqlrush/opendbx/internal/app", false},
	}
	for _, tc := range cases {
		got := violatesToolOnly(tc.path, toolOnly)
		if got != tc.want {
			t.Errorf("violatesToolOnly(%q): got %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestAllowSet(t *testing.T) {
	entries := []allowEntry{
		{Module: "a"}, {Module: "b"}, {Module: "c"},
	}
	s := allowSet(entries)
	for _, e := range entries {
		if _, ok := s[e.Module]; !ok {
			t.Errorf("missing %s in set", e.Module)
		}
	}
}

func TestAllowVersionSet(t *testing.T) {
	entries := []allowEntry{
		{Module: "a", Version: "v1"},
		{Module: "b", Version: "v2"},
	}
	s := allowVersionSet(entries)
	if s["a"] != "v1" || s["b"] != "v2" {
		t.Errorf("version set mismatch: %+v", s)
	}
}

// Smoke test against the real opendbx repo.
func TestCheck_RealRepo(t *testing.T) {
	root := "../../"
	violations, err := check(root, false)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(violations) > 0 {
		t.Errorf("real-repo check: %d violations:\n  %s", len(violations), strings.Join(violations, "\n  "))
	}
}
