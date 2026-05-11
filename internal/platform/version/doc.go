// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package version owns the build version string (read by cmd/opendbx --version)
// plus the canonical tag grammar + parser introduced in spec-0.7.
//
// Files currently present (T-3 D-1 scope):
//
//   - version.go: Version string + String() (set via linker -X).
//   - grammar.go: VersionPattern + Parse(tag, LookupFunc) — MINOR = global
//     cumulative spec ordinal (spec-registry SSOT, manifest authoritative;
//     spec-0.7 R2.1 Q13 ★B').
//   - errors.go: VERSION.TAG_INVALID sentinel (spec-0.6 errcode contract).
//
// Subsequent spec-0.7 tasks add to this package:
//
//   - T-4 D-3: Commit / BuildDate / Dirty vars + Verbose() multi-line block.
//
// Per spec-0.2 § 2.2, this is the **unique** cmd → platform exception (no
// other platform subpackage may be imported by cmd; everything else routes
// through entrypoints → bootstrap).
//
// Design: spec-0.7-version-numbering.
package version
