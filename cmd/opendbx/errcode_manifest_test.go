// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package main

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

// TestCodesFrozenManifest enforces the spec-0.6 § 3.2 stability contract:
// the set of registered (non-TEST.) codes must match the checked-in
// `internal/platform/errcode/testdata/error-codes-frozen.txt` manifest.
//
// This lives in cmd/opendbx because building the opendbx binary
// transitively imports every package that calls errcode.Register
// (entrypoints / config / logger via newRootCommand). Putting the test in
// internal/platform/errcode would create a cycle; putting it in tools/
// breaches layer rules (tools → platform / entrypoints not allowed).
//
// errcode is whitelisted via CmdPlatformExceptionPaths (spec-0.6 § 5.2)
// alongside platform/version.
//
// Runtime registration alone only covers packages reachable from newRootCommand.
// The static scan below catches packages such as render/* whose registered
// sentinels are real production API but are not imported by the CLI yet.
//
// codex MED-4 R2 enforcement.
func TestCodesFrozenManifest(t *testing.T) {
	// Side-effect: build the root command to trigger every package's
	// file-scope `var Err = Register(...)` registrations.
	_ = newRootCommand()

	const manifestPath = "../../internal/platform/errcode/testdata/error-codes-frozen.txt"
	manifest, err := readManifest(t, manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}

	live := errcode.All()
	liveSet := make(map[string]bool, len(live))
	for _, def := range live {
		liveSet[def.Code] = true
	}

	staticCodes, err := scanRegisteredCodes(t, "../..")
	if err != nil {
		t.Fatalf("scan registered codes: %v", err)
	}
	for _, code := range staticCodes {
		liveSet[code] = true
	}
	liveCodes := make([]string, 0, len(liveSet))
	for code := range liveSet {
		liveCodes = append(liveCodes, code)
	}
	sort.Strings(liveCodes)

	manifestSet := make(map[string]bool, len(manifest))
	for _, code := range manifest {
		manifestSet[code] = true
	}
	var removed, added []string
	for _, code := range manifest {
		if !liveSet[code] {
			removed = append(removed, code)
		}
	}
	for _, code := range liveCodes {
		if !manifestSet[code] {
			added = append(added, code)
		}
	}

	// NB (claude LOW-1 R2 alignment): a code rename shows up here as ONE
	// entry in removed and ONE in added. If you're seeing both errors after
	// a deliberate rename — that's expected; commit both the manifest update
	// and the rename together.
	if len(removed) > 0 {
		t.Errorf(
			"codes deleted vs frozen manifest (NOT ALLOWED — spec-0.6 § 3.2 stability):\n  %s\n"+
				"  if intentional deprecation: mark with // Deprecated: comment instead of removing\n"+
				"  (renames appear in both removed AND added lists; expected if you renamed)",
			strings.Join(removed, "\n  "),
		)
	}
	if len(added) > 0 {
		t.Errorf(
			"new codes added vs frozen manifest (please commit manifest update):\n  %s\n"+
				"  regenerate via `go run cmd/tools/gen-error-codes/main.go` then refresh\n"+
				"  internal/platform/errcode/testdata/error-codes-frozen.txt",
			strings.Join(added, "\n  "),
		)
	}
}

func scanRegisteredCodes(t *testing.T, relRoot string) ([]string, error) {
	t.Helper()
	root, err := filepath.Abs(relRoot)
	if err != nil {
		return nil, err
	}
	codes := make(map[string]bool)
	fset := token.NewFileSet()
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "vendor", "node_modules", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Register" {
				return true
			}
			ident, ok := sel.X.(*ast.Ident)
			if !ok || ident.Name != "errcode" {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			code, err := strconv.Unquote(lit.Value)
			if err == nil && code != "" && !strings.HasPrefix(code, "TEST.") {
				codes[code] = true
			}
			return true
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(codes))
	for code := range codes {
		out = append(out, code)
	}
	sort.Strings(out)
	return out, nil
}

func readManifest(t *testing.T, relPath string) ([]string, error) {
	t.Helper()
	// go-reviewer M-4 R2 alignment: filepath.Clean does NOT make the path
	// absolute, only normalises ".." / "./". Use filepath.Abs so the test
	// works regardless of the cwd the test binary runs from (CI, IDE,
	// `cd ../.. && go test ./cmd/opendbx/...` etc.).
	abs, err := filepath.Abs(relPath)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(abs)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines, scanner.Err()
}
