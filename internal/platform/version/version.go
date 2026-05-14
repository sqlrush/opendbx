// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Build metadata vars + Verbose() multi-line block (spec-0.7 D-3).
// Package overview lives in doc.go.

package version

import (
	"fmt"
	"runtime"
)

// Build metadata vars — set by the linker via -X (T-5 wires the Makefile
// injection). Defaults are dev/unknown so `go run` and `go test` still
// work without any build step.
//
//nolint:gochecknoglobals // spec-0.7 D-3: linker -X injection requires package-level vars.
var (
	// Version is the build version string (typically a tag from
	// `git describe --tags --always`; see Makefile spec-0.7 § 2.3 T-5).
	Version = "dev"

	// Commit is the 12-char short hash of the build HEAD
	// (`git rev-parse --short=12 HEAD`).
	Commit = "unknown"

	// BuildDate is the UTC ISO 8601 build timestamp
	// (`date -u +%Y-%m-%dT%H:%M:%SZ`).
	BuildDate = "unknown"

	// Dirty reflects working-tree state at build time. Empty string =
	// clean; "dirty" = uncommitted changes (tracked OR untracked, captured
	// via `git status --porcelain` per spec-0.7 § 2.3 codex MED fix).
	Dirty = ""
)

// String returns just Version (CC parity; used by `opendbx --version`).
func String() string { return Version }

// Verbose returns the multi-line block emitted by `opendbx --version-verbose`
// (T-6 wires the cobra flag). Format is spec-0.7 § 2.2:
//
//	opendbx <Version>
//	commit:     <Commit>
//	built:      <BuildDate>
//	workdir:    clean|dirty
//	go:         <runtime.Version()>
//	os/arch:    <runtime.GOOS>/<runtime.GOARCH>
//
// workdir resolves to literal "clean" or "dirty" (claude MED-5 fix: avoid
// ternary-like format text that readers parse backwards).
func Verbose() string {
	workdir := "clean"
	if Dirty == "dirty" {
		workdir = "dirty"
	}
	return fmt.Sprintf(
		"opendbx %s\ncommit:     %s\nbuilt:      %s\nworkdir:    %s\ngo:         %s\nos/arch:    %s/%s\n",
		Version, Commit, BuildDate, workdir, runtime.Version(), runtime.GOOS, runtime.GOARCH,
	)
}
