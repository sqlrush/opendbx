// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package llmapp

import (
	"strings"
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/block"
	"github.com/sqlrush/opendbx/internal/domain/llm/fake"
)

// TestModel_View_WelcomeHeadVisible: a freshly seeded welcome (scrollback[0])
// is visible in View when the scrollback is short. This is the spec-1.25 D-8
// regression surface — the live render path is Model.View's bottom-up loop
// over the plain []block.RenderNode slice (NOT render/scrollback's
// VirtualScrollback, which is never instantiated in production / CRIT-C).
func TestModel_View_WelcomeHeadVisible(t *testing.T) {
	t.Parallel()
	m := New(fake.New(), Options{Welcome: true, Version: "v0.49.0", Cwd: "~/opendbx"})
	txt := gridText(m.View(80, 24))
	if !strings.Contains(txt, "Welcome to opendbx") {
		t.Errorf("View should show the seeded welcome; got:\n%s", txt)
	}
}

// TestModel_View_LargeScrollbackClipsHead: with a tall scrollback the
// bottom-up loop clips the welcome head (y<0 short-circuit) and renders the
// tail without panic or overflow. Guards the D-8 perf/offset claim that the
// welcome at scrollback[0] does not get fully re-rendered every frame.
func TestModel_View_LargeScrollbackClipsHead(t *testing.T) {
	t.Parallel()
	m := New(fake.New(), Options{Welcome: true, Version: "v0.49.0", Cwd: "~/x"})
	for i := 0; i < 5000; i++ {
		m.scrollback = append(m.scrollback, block.Message{Text: "line"})
	}
	buf := m.View(80, 24)
	if buf == nil {
		t.Fatal("View returned nil on large scrollback")
	}
	cols, rows := buf.Size()
	if cols != 80 || rows != 24 {
		t.Errorf("View size = (%d,%d); want (80,24)", cols, rows)
	}
	// the welcome head is clipped (not visible) when scrollback is tall
	if strings.Contains(gridText(buf), "Welcome to opendbx") {
		t.Errorf("welcome head should be clipped off-screen with 5000 lines")
	}
}
