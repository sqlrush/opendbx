// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package main

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
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
	liveCodes := make([]string, len(live))
	for i, def := range live {
		liveCodes[i] = def.Code
	}
	sort.Strings(liveCodes)

	manifestSet := make(map[string]bool, len(manifest))
	for _, code := range manifest {
		manifestSet[code] = true
	}
	liveSet := make(map[string]bool, len(liveCodes))
	for _, code := range liveCodes {
		liveSet[code] = true
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

	if len(removed) > 0 {
		t.Errorf(
			"codes deleted vs frozen manifest (NOT ALLOWED — spec-0.6 § 3.2 stability):\n  %s\n"+
				"  if intentional deprecation: mark with // Deprecated: comment instead of removing",
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

func readManifest(t *testing.T, relPath string) ([]string, error) {
	t.Helper()
	abs := filepath.Clean(relPath)
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
