// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// IMP-6 render-cascade (retrofit of spec-0.2 render strict DAG; spec-0.10
// D-3 R2 codex H5 修：原 spec-0.10 R1 描述"9 层"是 typo，实际 10 层).
//
// Render subpackage strict DAG (spec-0.2 § 2.2 重要细则 #3, CLAUDE.md § 3.1).
// 10 layers, ordered:
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

// renderClassify inspects an import path. Returns:
//
//	(idx, ok=true)            — path is under render/ AND its first segment
//	                            is in RenderOrder; idx is RenderOrder position.
//	(0, ok=false, unknown="x") — path is under render/ but first segment is
//	                            NOT in RenderOrder (unknown subpackage —
//	                            must be added to DAG before use).
//	(0, ok=false, unknown="")  — path is not under render/ at all.
func renderClassify(importPath string) (idx int, ok bool, unknown string) {
	if !strings.HasPrefix(importPath, RenderRoot) {
		return 0, false, ""
	}
	rel := strings.TrimPrefix(importPath, RenderRoot)
	first := firstSegment(rel)
	if first == "" {
		return 0, false, ""
	}
	for i, name := range RenderOrder {
		if first == name {
			return i, true, ""
		}
	}
	return 0, false, first
}

// CheckRenderDAG returns "" if the edge obeys the render strict DAG, or a
// violation reason. Behavior:
//   - Edges where neither endpoint is under render/ are ignored.
//   - If either endpoint is under render/ but its first segment is unknown
//     to RenderOrder, the edge fails (a new render subpackage must be
//     added to RenderOrder explicitly before it can be imported).
//   - Otherwise: idx_from < idx_to is allowed, idx_from >= idx_to fails.
func CheckRenderDAG(from, to string) string {
	fi, fOK, fUnknown := renderClassify(from)
	ti, tOK, tUnknown := renderClassify(to)

	// Both outside render/: this rule family doesn't care.
	if fUnknown == "" && !fOK && tUnknown == "" && !tOK {
		return ""
	}
	// Unknown render subpackage on either side: hard fail (force authors
	// to update RenderOrder before adding new render/* dirs).
	if fUnknown != "" {
		return fmt.Sprintf("render-DAG: from-package render/%s is not in RenderOrder — add it to RenderOrder (and update spec § 2.2) before using. Current order: %s",
			fUnknown, strings.Join(RenderOrder, " → "))
	}
	if tUnknown != "" {
		return fmt.Sprintf("render-DAG: imported package render/%s is not in RenderOrder — add it to RenderOrder (and update spec § 2.2) before using. Current order: %s",
			tUnknown, strings.Join(RenderOrder, " → "))
	}
	// Only one endpoint is in render/, the other is outside — no DAG
	// constraint applies (other rule families handle inter-layer).
	if !fOK || !tOK {
		return ""
	}
	if fi >= ti {
		return fmt.Sprintf("render-DAG: %s (idx %d) imports %s (idx %d) — must be strictly downward (idx_from < idx_to). Order: %s",
			RenderOrder[fi], fi, RenderOrder[ti], ti,
			strings.Join(RenderOrder, " → "))
	}
	return ""
}
