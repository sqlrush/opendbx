// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package scrollback maintains the long-history scrollback of rendered
// block.RenderNode items. spec-0.13 D-1 ships interface only;
// spec-1.5-virtual-scrollback adds offset table + binary search + overscan.
//
// DAG position: render/scrollback is index 8 (depends on render/buffer +
// render/layout + render/block — R2 CRIT-1 reordering moved block to
// index 7 so scrollback can legally import it).
//
// Design: spec-0.13-render-engine-skeleton § 2.1 (D-1)
package scrollback

import (
	"github.com/sqlrush/opendbx/internal/app/cli/render/block"
)

// Scrollback is the long-history container.
type Scrollback interface {
	Push(node block.RenderNode)
	Range(start, end int) []block.RenderNode
	Len() int
}
