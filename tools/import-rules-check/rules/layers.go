// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package rules implements opendbx's layered import-direction rules
// (spec-0.2 § 2.2). Pure data + pure functions; no side effects.
package rules

import (
	"fmt"
	"strings"
)

// ModulePrefix is the canonical opendbx module path prefix.
const ModulePrefix = "github.com/sqlrush/opendbx/"

// Layer enumerates the dependency layers tracked by spec-0.2.
type Layer int

// Layer values: order is internal; only equality and the LayerMatrix
// lookups are observable.
const (
	LayerStdlib Layer = iota
	LayerExternal
	LayerCmd
	LayerEntrypoints
	LayerBootstrap
	LayerApp
	LayerDomain
	LayerPlatform
	LayerTests
	LayerTools
	LayerPkg
	LayerUnknown
)

func (l Layer) String() string {
	switch l {
	case LayerStdlib:
		return "stdlib"
	case LayerExternal:
		return "external"
	case LayerCmd:
		return "cmd"
	case LayerEntrypoints:
		return "entrypoints"
	case LayerBootstrap:
		return "bootstrap"
	case LayerApp:
		return "app"
	case LayerDomain:
		return "domain"
	case LayerPlatform:
		return "platform"
	case LayerTests:
		return "tests"
	case LayerTools:
		return "tools"
	case LayerPkg:
		return "pkg"
	default:
		return "unknown"
	}
}

// Stdlib detection — Go stdlib import paths never contain a dot before the
// first slash (e.g., "fmt", "encoding/json"). Module paths always have a dot
// in the first segment (e.g., "github.com/...", "golang.org/x/...").
func isStdlib(importPath string) bool {
	first := importPath
	if idx := strings.IndexByte(importPath, '/'); idx >= 0 {
		first = importPath[:idx]
	}
	return !strings.ContainsRune(first, '.')
}

// PathToLayer classifies an import path into a Layer.
func PathToLayer(importPath string) Layer {
	if isStdlib(importPath) {
		return LayerStdlib
	}
	if !strings.HasPrefix(importPath, ModulePrefix) {
		return LayerExternal
	}
	rel := strings.TrimPrefix(importPath, ModulePrefix)
	switch {
	case rel == "cmd" || strings.HasPrefix(rel, "cmd/"):
		return LayerCmd
	case rel == "tools" || strings.HasPrefix(rel, "tools/"):
		return LayerTools
	case rel == "pkg" || strings.HasPrefix(rel, "pkg/"):
		return LayerPkg
	case rel == "tests" || strings.HasPrefix(rel, "tests/"):
		return LayerTests
	case rel == "internal/bootstrap" || strings.HasPrefix(rel, "internal/bootstrap/"):
		return LayerBootstrap
	case rel == "internal/entrypoints" || strings.HasPrefix(rel, "internal/entrypoints/"):
		return LayerEntrypoints
	case strings.HasPrefix(rel, "internal/app/"):
		return LayerApp
	case strings.HasPrefix(rel, "internal/domain/"):
		return LayerDomain
	case strings.HasPrefix(rel, "internal/platform/"):
		return LayerPlatform
	default:
		return LayerUnknown
	}
}

// CmdPlatformExceptionPaths lists every platform subpackage that `cmd/...`
// is permitted to import. Exact-match only — sub-packages like
// `internal/platform/version/build` do NOT satisfy an exception.
//
// Per spec § 2.2 + spec-0.3 § 1.4 + D-9, the allow list is intentionally
// small. Adding a new exception requires its own spec section.
//
//   - internal/platform/version (spec-0.2 § 2.2): main.go embeds the build
//     version string set via linker flag.
//   - internal/platform/profileutil (spec-0.3 D-9): main.go records a
//     startup checkpoint before any other code can run, parallel to CC
//     main.tsx L1 `profileCheckpoint('main_tsx_entry')`.
var CmdPlatformExceptionPaths = []string{
	ModulePrefix + "internal/platform/version",
	ModulePrefix + "internal/platform/profileutil",
}

// CmdPlatformVersionExceptionPath retained for back-compat (deprecated).
//
// Deprecated: use CmdPlatformExceptionPaths.
const CmdPlatformVersionExceptionPath = ModulePrefix + "internal/platform/version"

// MigrationsPath is the platform/migrations path; only bootstrap may
// import it (spec-0.2 § 2.2 重要细则 #1). Path-boundary safe: matches the
// exact path or any sub-path under it (e.g. migrations/sql), but NOT
// sibling paths like migrations2 or migrations-test.
const MigrationsPath = ModulePrefix + "internal/platform/migrations"

// SchemasPath is the domain/schemas path; spec § 2.2 重要细则 #2 declares
// it global-read (any layer may import). Pure data, no behavior.
const SchemasPath = ModulePrefix + "internal/domain/schemas"

// pathHasBoundary returns true when target equals prefix exactly, or target
// is a sub-path under prefix (boundary-safe; rejects sibling like
// `<prefix>2` or `<prefix>-foo`).
func pathHasBoundary(target, prefix string) bool {
	return target == prefix || strings.HasPrefix(target, prefix+"/")
}

// LayerMatrix encodes the allowed from→to layer transitions
// (spec-0.2 § 2.2 matrix). Read as: layerMatrix[fromLayer][toLayer] == true
// → import allowed. Special-cases (cmd → platform/version, migrations
// gating) live in CheckEdge below.
var LayerMatrix = map[Layer]map[Layer]bool{
	LayerCmd: {
		LayerStdlib:      true,
		LayerEntrypoints: true,
		// LayerPlatform handled as special case (only platform/version)
	},
	LayerEntrypoints: {
		LayerStdlib:    true,
		LayerBootstrap: true,
		LayerPlatform:  true,
	},
	LayerBootstrap: {
		LayerStdlib:   true,
		LayerApp:      true,
		LayerDomain:   true,
		LayerPlatform: true,
	},
	LayerApp: {
		LayerStdlib:   true,
		LayerApp:      true,
		LayerDomain:   true,
		LayerPlatform: true,
	},
	LayerDomain: {
		LayerStdlib:   true,
		LayerDomain:   true,
		LayerPlatform: true,
	},
	LayerPlatform: {
		LayerStdlib:   true,
		LayerPlatform: true,
	},
	LayerTests: {
		LayerStdlib:      true,
		LayerEntrypoints: true,
		LayerBootstrap:   true,
		LayerApp:         true,
		LayerDomain:      true,
		LayerPlatform:    true,
	},
	LayerTools: {
		LayerStdlib:   true,
		LayerExternal: true,
		LayerTools:    true, // same-tool subpackages OK (e.g. tools/import-rules-check imports tools/import-rules-check/rules)
	},
	LayerPkg: {
		LayerStdlib:   true,
		LayerPlatform: true,
		LayerDomain:   true,
	},
}

// CheckEdge returns "" if the from→to import is allowed, or a violation
// reason. External (third-party) imports are not the responsibility of this
// tool — `dep-allowlist-check` handles those. Stdlib is always allowed.
func CheckEdge(from, to string) string {
	fromLayer := PathToLayer(from)
	toLayer := PathToLayer(to)

	// Stdlib is always allowed (regardless of from-layer).
	if toLayer == LayerStdlib {
		return ""
	}
	// External deps are dep-allowlist-check's domain.
	if toLayer == LayerExternal {
		return ""
	}

	// schemas global-read exception (spec § 2.2 重要细则 #2): pure data
	// package, any layer may import (including platform).
	if pathHasBoundary(to, SchemasPath) {
		return ""
	}

	// migrations gating: only bootstrap may import platform/migrations.
	// Boundary-safe: matches platform/migrations and platform/migrations/sub
	// but not platform/migrations2.
	if pathHasBoundary(to, MigrationsPath) && fromLayer != LayerBootstrap {
		return fmt.Sprintf("internal/platform/migrations may only be imported by internal/bootstrap (got %s)", layerOrPath(fromLayer, from))
	}

	// cmd → platform: only the exact paths in CmdPlatformExceptionPaths
	// are allowed (sub-paths do NOT qualify).
	if fromLayer == LayerCmd && toLayer == LayerPlatform {
		for _, allowed := range CmdPlatformExceptionPaths {
			if to == allowed {
				return ""
			}
		}
		return fmt.Sprintf("cmd may import only %v (got %s)", CmdPlatformExceptionPaths, to)
	}

	if !LayerMatrix[fromLayer][toLayer] {
		return fmt.Sprintf("layer %s → %s is not allowed", fromLayer, toLayer)
	}
	return ""
}

// layerOrPath provides a friendlier "from" string for error messages.
func layerOrPath(layer Layer, path string) string {
	if layer == LayerUnknown {
		return path
	}
	return fmt.Sprintf("%s (%s)", layer, path)
}
