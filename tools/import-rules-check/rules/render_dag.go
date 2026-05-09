// Copyright 2026 opendbx contributors. See LICENSE.
//
// Render subpackage strict DAG (spec-0.2 § 2.2 重要细则 #3, CLAUDE.md § 3.1).
//
// Order:
//
//	terminal → buffer → layout → optimizer → scheduler → scrollback → streaming → block → style → width
//
// Direction semantics (依赖方向单向向下):
//   - "X → Y" means X imports Y (X depends on Y).
//   - X is at index i, Y at index j; allowed iff i < j (importing strictly later in list).
//   - Reverse direction (i >= j) is FORBIDDEN.
//
// Examples:
//   - terminal (0) imports buffer (1):    OK (0 < 1)
//   - terminal (0) imports width (9):     OK (0 < 9; long jump is fine, still downward)
//   - block (7) imports scheduler (4):    FAIL (7 >= 4, upward in list)
//   - buffer (1) imports buffer (1):      FAIL (Go disallows self-import anyway, but rule says i < j)
//
// Author: sqlrush
package rules

import (
	"fmt"
	"strings"
)

// RenderRoot is the package path prefix for the render subsystem.
const RenderRoot = ModulePrefix + "internal/app/cli/render/"

// RenderOrder is the canonical strict DAG order. Earlier entries may import
// later entries; later entries may NOT import earlier entries.
var RenderOrder = []string{
	"terminal",
	"buffer",
	"layout",
	"optimizer",
	"scheduler",
	"scrollback",
	"streaming",
	"block",
	"style",
	"width",
}

// renderIndex returns the position of a render-subpackage path in
// RenderOrder, or -1 if path is not within the render subsystem (or is the
// render root itself).
func renderIndex(importPath string) int {
	if !strings.HasPrefix(importPath, RenderRoot) {
		return -1
	}
	rel := strings.TrimPrefix(importPath, RenderRoot)
	first := firstSegment(rel)
	if first == "" {
		return -1
	}
	for i, name := range RenderOrder {
		if first == name {
			return i
		}
	}
	return -1
}

// CheckRenderDAG returns "" if the edge obeys the render strict DAG, or a
// violation reason. Edges where either endpoint is outside render/ are
// ignored (other rule families handle those).
func CheckRenderDAG(from, to string) string {
	fi := renderIndex(from)
	ti := renderIndex(to)
	if fi < 0 || ti < 0 {
		return ""
	}
	if fi >= ti {
		return fmt.Sprintf("render-DAG: %s (idx %d) imports %s (idx %d) — must be strictly downward (idx_from < idx_to). Order: %s",
			RenderOrder[fi], fi, RenderOrder[ti], ti,
			strings.Join(RenderOrder, " → "))
	}
	return ""
}
