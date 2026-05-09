// Copyright 2026 opendbx contributors. See LICENSE.
//
// dep-allowlist-check: validates the opendbx module's dependency graph
// against docs/dependencies/allowlist.yml (spec-0.2 § 2.4 / § 2.5, D-6).
//
// Three rules:
//
//  1. Every direct require in go.mod must be listed under
//     `direct_allowed:` in allowlist.yml. Modules listed there must include
//     `introduced_by: spec-X.Y` referencing the spec that approved them.
//  2. Every indirect (transitive) module must be listed under
//     `transitive_lock:` (with version). New transitive arrivals fail CI
//     and require human review (update lock atomically with the dep change).
//  3. Modules listed under `tool_only:` may only be imported by packages
//     under `tools/`. Imports from `cmd/`, `internal/`, `tests/`, or `pkg/`
//     are violations.
//
// Author: sqlrush
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
	"gopkg.in/yaml.v3"
)

const usage = `dep-allowlist-check [-v] [<repo-root>]

Validates the opendbx module dependency graph per spec-0.2 § 2.4.

Flags:
  -v        verbose
  <root>    opendbx repo root (default: current directory)
`

const modulePrefix = "github.com/sqlrush/opendbx/"

type allowEntry struct {
	Module       string `yaml:"module"`
	Purpose      string `yaml:"purpose"`
	IntroducedBy string `yaml:"introduced_by"`
	Version      string `yaml:"version"` // only used by transitive_lock
}

type allowlist struct {
	DirectAllowed  []allowEntry `yaml:"direct_allowed"`
	TransitiveLock []allowEntry `yaml:"transitive_lock"`
	ToolOnly       []allowEntry `yaml:"tool_only"`
}

type goModule struct {
	Path     string `json:"Path"`
	Version  string `json:"Version"`
	Main     bool   `json:"Main"`
	Indirect bool   `json:"Indirect"`
}

func main() {
	var verbose bool
	flag.BoolVar(&verbose, "v", false, "verbose")
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	flag.Parse()

	root := flag.Arg(0)
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			fmt.Fprintln(os.Stderr, "getwd:", err)
			os.Exit(2)
		}
	}

	violations, err := check(root, verbose)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dep-allowlist-check:", err)
		os.Exit(2)
	}

	if len(violations) > 0 {
		fmt.Fprintln(os.Stderr, "dep-allowlist-check FAIL:")
		sort.Strings(violations)
		for _, v := range violations {
			fmt.Fprintln(os.Stderr, "  -", v)
		}
		os.Exit(1)
	}
	if verbose {
		fmt.Println("dep-allowlist-check OK")
	}
}

func check(root string, verbose bool) ([]string, error) {
	allow, err := loadAllowlist(filepath.Join(root, "docs", "dependencies", "allowlist.yml"))
	if err != nil {
		return nil, fmt.Errorf("load allowlist: %w", err)
	}

	// Read go list -m -json all from the repo root.
	mods, err := goListModules(root)
	if err != nil {
		return nil, fmt.Errorf("go list: %w", err)
	}

	var violations []string

	directApproved := allowSet(allow.DirectAllowed)
	transitiveApproved := allowVersionSet(allow.TransitiveLock)
	toolOnlySet := allowSet(allow.ToolOnly)

	for _, m := range mods {
		if m.Main {
			continue
		}
		// direct vs indirect
		if m.Indirect {
			// transitive: must be in transitive_lock with matching version.
			lockedVer, ok := transitiveApproved[m.Path]
			if !ok {
				violations = append(violations, fmt.Sprintf("transitive module %s@%s not in transitive_lock (run human review then add to allowlist.yml)", m.Path, m.Version))
				continue
			}
			if lockedVer != "" && lockedVer != m.Version {
				violations = append(violations, fmt.Sprintf("transitive module %s version drift: locked=%s actual=%s", m.Path, lockedVer, m.Version))
			}
		} else {
			// direct: must be in direct_allowed OR tool_only (tool_only modules
			// are direct requires too — gopkg.in/yaml.v3 / golang.org/x/tools).
			if _, ok := directApproved[m.Path]; ok {
				continue
			}
			if _, ok := toolOnlySet[m.Path]; ok {
				continue
			}
			violations = append(violations, fmt.Sprintf("direct require %s not in direct_allowed or tool_only (add to allowlist.yml + reference introducing spec)", m.Path))
		}
	}

	// tool_only enforcement: scan opendbx packages, fail if non-tools package
	// imports a tool_only module.
	toolOnlyViolations, err := checkToolOnly(root, toolOnlySet)
	if err != nil {
		return nil, fmt.Errorf("check tool_only: %w", err)
	}
	violations = append(violations, toolOnlyViolations...)

	if verbose {
		fmt.Printf("modules: %d (direct allowlisted: %d, tool_only: %d, transitive locked: %d)\n",
			len(mods), len(directApproved), len(toolOnlySet), len(transitiveApproved))
	}
	return violations, nil
}

// loadAllowlist reads and validates the allowlist.yml file.
func loadAllowlist(path string) (*allowlist, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var a allowlist
	if err := yaml.Unmarshal(raw, &a); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	// Validate every entry has a module path.
	for _, e := range a.DirectAllowed {
		if e.Module == "" || e.IntroducedBy == "" {
			return nil, fmt.Errorf("direct_allowed entry must have module + introduced_by; got %+v", e)
		}
	}
	for _, e := range a.TransitiveLock {
		if e.Module == "" || e.Version == "" {
			return nil, fmt.Errorf("transitive_lock entry must have module + version; got %+v", e)
		}
	}
	for _, e := range a.ToolOnly {
		if e.Module == "" || e.IntroducedBy == "" {
			return nil, fmt.Errorf("tool_only entry must have module + introduced_by; got %+v", e)
		}
	}
	return &a, nil
}

// goListModules invokes `go list -m -json all` and parses the streaming
// JSON object output.
func goListModules(root string) ([]goModule, error) {
	cmd := exec.Command("go", "list", "-m", "-json", "all")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		// Surface stderr if available
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("%w: %s", err, string(exitErr.Stderr))
		}
		return nil, err
	}
	dec := json.NewDecoder(strings.NewReader(string(out)))
	var mods []goModule
	for dec.More() {
		var m goModule
		if err := dec.Decode(&m); err != nil {
			return nil, fmt.Errorf("decode go list output: %w", err)
		}
		mods = append(mods, m)
	}
	return mods, nil
}

// checkToolOnly walks all opendbx packages; any package outside tools/
// importing a tool_only module fails.
func checkToolOnly(root string, toolOnly map[string]struct{}) ([]string, error) {
	if len(toolOnly) == 0 {
		return nil, nil
	}
	cfg := &packages.Config{
		Mode:  packages.NeedName | packages.NeedFiles | packages.NeedImports,
		Dir:   root,
		Tests: false,
	}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return nil, err
	}
	var loadErrs []string
	for _, p := range pkgs {
		for _, e := range p.Errors {
			loadErrs = append(loadErrs, fmt.Sprintf("%s: %s", p.PkgPath, e))
		}
	}
	if len(loadErrs) > 0 {
		return nil, fmt.Errorf("packages.Load returned errors:\n  %s", strings.Join(loadErrs, "\n  "))
	}

	var violations []string
	for _, pkg := range pkgs {
		if !strings.HasPrefix(pkg.PkgPath, modulePrefix) {
			continue
		}
		// Allowed locations for tool_only modules: anything under tools/.
		isTools := strings.HasPrefix(strings.TrimPrefix(pkg.PkgPath, modulePrefix), "tools/")
		if isTools {
			continue
		}
		for _, imp := range pkg.Imports {
			if violatesToolOnly(imp.PkgPath, toolOnly) {
				violations = append(violations, fmt.Sprintf("non-tools package %s imports tool_only module path %s", pkg.PkgPath, imp.PkgPath))
			}
		}
	}
	return violations, nil
}

// violatesToolOnly returns true if the import path begins with any tool_only
// module path.
func violatesToolOnly(importPath string, toolOnly map[string]struct{}) bool {
	for mod := range toolOnly {
		if importPath == mod || strings.HasPrefix(importPath, mod+"/") {
			return true
		}
	}
	return false
}

func allowSet(entries []allowEntry) map[string]struct{} {
	m := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		m[e.Module] = struct{}{}
	}
	return m
}

func allowVersionSet(entries []allowEntry) map[string]string {
	m := make(map[string]string, len(entries))
	for _, e := range entries {
		m[e.Module] = e.Version
	}
	return m
}
