// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// IMP-6 render-cascade — render subpackage strict DAG (spec-0.2 § 2.2
// 重要细则 #3, CLAUDE.md § 3.1, spec-0.13 D-4 BREAKING retrofit).
//
// **spec-0.13 R2 CRIT-1 BREAKING retrofit (2026-05-16)**: the original
// spec-0.2 retrofit (spec-0.10 D-3) had RenderOrder reversed — listed
// terminal as index 0 (top caller) and width as index 9 (leaf). That
// did NOT match § 2.1 actual package dependencies, where width/style
// are leaves and block/streaming are roots. R2 sequence reverses to
// leaf→root + comparison operator flips from `fi >= ti` to `fi <= ti`.
//
// Render subpackage strict DAG (leaf→root, 10 layers):
//
//	width → style → terminal → buffer → layout → optimizer → scheduler → block → scrollback → streaming
//	(0)     (1)      (2)        (3)       (4)      (5)         (6)         (7)     (8)          (9)
//
// Direction semantics (依赖方向高 index → 低 index):
//   - "X imports Y" allowed iff index(X) > index(Y).
//   - Reverse direction (idx_from <= idx_to) is FORBIDDEN (cycle / leaf
//     reaching back upward into root callers).
//
// Examples (post-spec-0.13):
//   - block (7) imports layout (4):     OK (7 > 4, root reaches leaf)
//   - block (7) imports width (0):      OK (7 > 0)
//   - scrollback (8) imports block (7): OK (8 > 7)
//   - streaming (9) imports block (7):  OK (9 > 7)
//   - layout (4) imports block (7):     FAIL (4 <= 7, leaf cannot reach root)
//   - block (7) imports scrollback (8): FAIL (7 <= 8, root cannot reach higher root)
//
// Cross-spec errata: spec-0.10 § D-3 + spec-0.2 § 2.2 重要细则 #3 +
// CLAUDE.md § 3.1 inline errata 段 引用本 spec-0.13 D-4. CLAUDE.md
// 走 § 9 完整修订协议. errata 不打 patch tag (spec-0.7 errata 协议).

package rules

import (
	"fmt"
	"strings"
)

// RenderRoot is the package path prefix for the render subsystem.
const RenderRoot = ModulePrefix + "internal/app/cli/render/"

// RenderOrder is the canonical strict DAG order (leaf→root post spec-0.13).
// Higher-index entries may import lower-index entries; the reverse is
// FORBIDDEN. See package godoc for rationale.
var RenderOrder = []string{
	"width",      // 0 — leaf: pure utility, no internal deps
	"style",      // 1 — leaf: pure data + ANSI generation
	"terminal",   // 2 — depends on style
	"buffer",     // 3 — depends on style + width
	"layout",     // 4 — depends on width
	"optimizer",  // 5 — depends on buffer + terminal
	"scheduler",  // 6 — depends on optimizer + terminal
	"block",      // 7 — intermediate root: depends on layout + buffer + width + style
	"scrollback", // 8 — depends on buffer + layout + block
	"streaming",  // 9 — true root: depends on scrollback + block
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
// violation reason. Behavior (spec-0.13 R2 BREAKING retrofit):
//   - Edges where neither endpoint is under render/ are ignored.
//   - If either endpoint is under render/ but its first segment is unknown
//     to RenderOrder, the edge fails (a new render subpackage must be
//     added to RenderOrder explicitly before it can be imported).
//   - Otherwise: idx_from > idx_to is allowed (root imports leaf);
//     idx_from <= idx_to fails (leaf cannot reach root, no self-import).
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
		return fmt.Sprintf("render-DAG: from-package render/%s is not in RenderOrder — add it to RenderOrder (and update spec § 2.2) before using. Current order (leaf→root): %s",
			fUnknown, strings.Join(RenderOrder, " → "))
	}
	if tUnknown != "" {
		return fmt.Sprintf("render-DAG: imported package render/%s is not in RenderOrder — add it to RenderOrder (and update spec § 2.2) before using. Current order (leaf→root): %s",
			tUnknown, strings.Join(RenderOrder, " → "))
	}
	// Only one endpoint is in render/, the other is outside — no DAG
	// constraint applies (other rule families handle inter-layer).
	if !fOK || !tOK {
		return ""
	}
	if fi <= ti {
		return fmt.Sprintf("render-DAG: %s (idx %d) imports %s (idx %d) — must be strictly upward in dependency (idx_from > idx_to). Order (leaf→root): %s",
			RenderOrder[fi], fi, RenderOrder[ti], ti,
			strings.Join(RenderOrder, " → "))
	}
	return ""
}
