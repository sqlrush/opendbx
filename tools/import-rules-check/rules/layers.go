// Copyright 2026 opendbx contributors. See LICENSE.
//
// Package rules implements opendbx's layered import-direction rules
// (spec-0.2 § 2.2). Pure data + pure functions; no side effects.
//
// Author: sqlrush
package rules

import (
	"fmt"
	"strings"
)

// ModulePrefix is the canonical opendbx module path prefix.
const ModulePrefix = "github.com/sqlrush/opendbx/"

// Layer enumerates the dependency layers tracked by spec-0.2.
type Layer int

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

// CmdPlatformVersionExceptionPath is the unique platform subpackage that
// `cmd/...` is permitted to import (spec-0.2 § 2.2). Everything else under
// internal/platform/* is off-limits to cmd; cmd must route through
// entrypoints → bootstrap.
const CmdPlatformVersionExceptionPath = ModulePrefix + "internal/platform/version"

// MigrationsPathPrefix is the platform/migrations path; only bootstrap may
// import it (spec-0.2 § 2.2 重要细则 #1).
const MigrationsPathPrefix = ModulePrefix + "internal/platform/migrations"

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
	// External deps are dep-allowlist-check's domain (except: internal/* must
	// not import external — that's caught by tier rules in dep-allowlist-check).
	// Tools are allowed to import external (e.g., golang.org/x/tools).
	if toLayer == LayerExternal {
		// internal code never imports external directly without going via tier rules.
		// This binary doesn't enforce that — dep-allowlist-check does. So pass.
		return ""
	}

	// migrations gating: only bootstrap may import platform/migrations
	if strings.HasPrefix(to, MigrationsPathPrefix) && fromLayer != LayerBootstrap {
		return fmt.Sprintf("internal/platform/migrations may only be imported by internal/bootstrap (got %s)", layerOrPath(fromLayer, from))
	}

	// cmd → platform: only platform/version is allowed.
	if fromLayer == LayerCmd && toLayer == LayerPlatform {
		if to == CmdPlatformVersionExceptionPath {
			return ""
		}
		return fmt.Sprintf("cmd may import only %s (got %s)", CmdPlatformVersionExceptionPath, to)
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
