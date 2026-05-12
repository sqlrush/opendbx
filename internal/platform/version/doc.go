// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package version owns the build version string (read by cmd/opendbx
// --version), the canonical tag grammar + parser, and the build metadata
// vars set via -X ldflags. All landed in spec-0.7.
//
// Files (spec-0.7 D-1 + D-3):
//
//   - version.go: Version + Commit + BuildDate + Dirty vars (set via -X
//     ldflags; defaults are dev/unknown) + String() + Verbose() multi-line
//     diagnostic block.
//   - grammar.go: VersionPattern (const string) + versionRegex (unexported
//     compiled regex) + Parse(tag, LookupFunc) (Info, error) + Info.String()
//     round-trip. MINOR = global cumulative spec ordinal (spec-registry
//     SSOT, manifest authoritative — spec-0.7 R2.1 Q13 ★B').
//   - errors.go: VERSION.TAG_INVALID errcode.Sentinel (spec-0.6 contract).
//
// Per spec-0.2 § 2.2, this is the **unique** cmd → platform exception (no
// other platform subpackage may be imported by cmd; everything else routes
// through entrypoints → bootstrap).
//
// Design: spec-0.7-version-numbering.
package version
