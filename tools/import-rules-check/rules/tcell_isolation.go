// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// IMP-9 tcell-isolation: forbid imports of github.com/gdamore/tcell/v2
// outside the two whitelisted packages (terminal probe + tui main loop).
//
// Rationale (CLAUDE.md § 3.1 + AD-002):
//
// opendbx renders on tcell, but the rest of the codebase must only see
// the abstractions exposed by `internal/app/cli/tui` (NewScreen factory)
// and `internal/platform/terminal` (Probe / IsInteractiveTTY). Letting
// arbitrary packages reach for tcell symbols defeats AD-002's "self-built
// engine on top of tcell" boundary and makes spec-1.x render subsystem
// extension harder.
//
// `*_test.go` files are exempt — test hooks may legitimately accept
// `tcell.Screen` parameters (see spec-0.12 § 2.3 root_test.go hook).

package rules

import (
	"fmt"
	"strings"
)

// TcellModule is the single tcell module root (v2 major).
const TcellModule = "github.com/gdamore/tcell/v2"

// TcellAllowedPrefixes are the only opendbx package prefixes permitted
// to import tcell from production source. spec-0.12 R3 H-4 + T-13 L-5
// reconciliation: original spec letter said 2 packages (terminal + tui),
// but the T-6 layer matrix forces a 3rd entry: bootstrap. Layer chain
// cmd → entrypoints → bootstrap → app/cli/tui is the only legal route
// for cmd to reach tui (LayerCmd matrix allows only Entrypoints; only
// LayerBootstrap allows LayerApp). bootstrap's runTUI signature takes
// tcell.Screen so bootstrap MUST be in this whitelist. spec § 2.5 +
// § 12 history T-13 entry record this reconciliation.
var TcellAllowedPrefixes = []string{
	ModulePrefix + "internal/platform/terminal",
	ModulePrefix + "internal/app/cli/tui",
	ModulePrefix + "internal/bootstrap",
}

// hasTcellPrefix reports whether `to` matches the tcell module root
// (boundary-safe — exact module or proper subpath only).
func hasTcellPrefix(to string) bool {
	return to == TcellModule || strings.HasPrefix(to, TcellModule+"/")
}

// isTcellAllowedSource reports whether `from` is in the strict 2-package
// production whitelist.
func isTcellAllowedSource(from string) bool {
	for _, p := range TcellAllowedPrefixes {
		if from == p || strings.HasPrefix(from, p+"/") {
			return true
		}
	}
	return false
}

// CheckTcellIsolation returns "" if the from→to edge is allowed, or a
// violation message describing the rule trip. Caller is responsible
// for skipping `*_test.go` files (test imports are exempt).
func CheckTcellIsolation(from, to string) string {
	if !hasTcellPrefix(to) {
		return ""
	}
	if isTcellAllowedSource(from) {
		return ""
	}
	return fmt.Sprintf(
		"IMP-9 tcell-isolation: %q imports tcell %q; only internal/platform/terminal + internal/app/cli/tui may import tcell from production source (CLAUDE.md § 3.1 + AD-002 self-built engine boundary)",
		from, to)
}
