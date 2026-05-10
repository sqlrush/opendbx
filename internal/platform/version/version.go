// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package version exposes the build version string. Set via linker flag:
//
//	-X github.com/sqlrush/opendbx/internal/platform/version.Version=<value>
//
// cmd/opendbx imports this package directly. Per spec-0.2 § 2.2, this is
// the **unique** cmd → platform exception (no other platform subpackage
// may be imported by cmd; everything else routes through entrypoints →
// bootstrap).
package version

// Version is set by the linker via -X. Defaults to "dev" for unreleased builds.
var Version = "dev"

// String returns the build version.
func String() string {
	return Version
}
