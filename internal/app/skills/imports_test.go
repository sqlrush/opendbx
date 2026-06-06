// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File imports_test.go — machine-enforced import allowlists (spec-2.3
// D-8, hardening the spec-2.2 D-9 leaf invariant):
//
//   - app/skills (this package) imports ONLY errcode + yaml + stdlib.
//     It must NOT import diagnose/config/logger/render — and must NOT
//     import its own invoke subpackage either (Go allows a parent
//     importing its child; the leaf rule is directional and needs an
//     explicit gate — spec-2.3 R2 arch MED-4).
//   - app/skills/invoke imports ONLY skills + diagnose + llm + errcode
//     + stdlib.

package skills

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const modPrefix = "github.com/sqlrush/opendbx/"

// nonStdImports returns the non-stdlib imports of every non-test .go
// file directly inside dir (no recursion).
func nonStdImports(t *testing.T, dir string) map[string][]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", dir, err)
	}
	out := map[string][]string{}
	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if strings.Contains(path, ".") { // stdlib has no dot in the first segment
				out[name] = append(out[name], path)
			}
		}
	}
	return out
}

// assertAllowlist fails for any import outside the allowed set.
func assertAllowlist(t *testing.T, byFile map[string][]string, allowed map[string]bool, scope string) {
	t.Helper()
	for file, imps := range byFile {
		for _, imp := range imps {
			if !allowed[imp] {
				t.Errorf("%s: %s imports %q — outside the %s allowlist", scope, file, imp, scope)
			}
		}
	}
}

// TestSkillsLeafImports — spec-2.2 D-9 invariant + spec-2.3 directional
// rule (the leaf never imports its invoke child).
func TestSkillsLeafImports(t *testing.T) {
	t.Parallel()
	allowed := map[string]bool{
		modPrefix + "internal/platform/errcode": true,
		"go.yaml.in/yaml/v3":                    true,
	}
	assertAllowlist(t, nonStdImports(t, "."), allowed, "app/skills leaf")
}

// TestInvokeImports — the invoke adapter may reach skills + diagnose +
// llm + errcode and nothing else (spec-2.3 D-8).
func TestInvokeImports(t *testing.T) {
	t.Parallel()
	allowed := map[string]bool{
		modPrefix + "internal/app/skills":       true,
		modPrefix + "internal/app/diagnose":     true,
		modPrefix + "internal/domain/llm":       true,
		modPrefix + "internal/platform/errcode": true,
	}
	assertAllowlist(t, nonStdImports(t, "invoke"), allowed, "app/skills/invoke")
}
