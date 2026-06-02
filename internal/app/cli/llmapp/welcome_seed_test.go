// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package llmapp

import (
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/block"
	"github.com/sqlrush/opendbx/internal/domain/llm/fake"
)

// TestWelcomeSeed_OnlyWhenEnabled is the spec-1.25 D-2 canary: a bare
// New (Welcome=false — the default for headless / error-fallback / test
// constructions) seeds NO welcome node, while Welcome=true seeds exactly
// one block.Welcome at scrollback head with the supplied version/cwd.
func TestWelcomeSeed_OnlyWhenEnabled(t *testing.T) {
	t.Parallel()

	// default: no welcome
	bare := New(fake.New(), Options{ModelName: "fake"})
	if len(bare.scrollback) != 0 {
		t.Errorf("bare New must not seed welcome; scrollback len=%d", len(bare.scrollback))
	}

	// interactive: welcome seeded at head
	withW := New(fake.New(), Options{
		ModelName: "fake",
		Welcome:   true,
		Version:   "v0.49.0",
		Cwd:       "~/opendbx",
		GitBranch: "spec-1.25",
	})
	if len(withW.scrollback) != 1 {
		t.Fatalf("Welcome=true must seed exactly one node; got %d", len(withW.scrollback))
	}
	w, ok := withW.scrollback[0].(block.Welcome)
	if !ok {
		t.Fatalf("scrollback[0] = %T; want block.Welcome", withW.scrollback[0])
	}
	if w.Version != "v0.49.0" || w.Cwd != "~/opendbx" {
		t.Errorf("welcome fields = %+v; want version/cwd populated", w)
	}
	// cwd + gitBranch are cached for the status bar (zero per-frame IO).
	if withW.cwd != "~/opendbx" || withW.gitBranch != "spec-1.25" {
		t.Errorf("status cache = (%q,%q); want (~/opendbx, spec-1.25)", withW.cwd, withW.gitBranch)
	}
}

// TestStatusSegments_RichChrome verifies the status bar carries model · cwd
// · git (spec-1.25 D-4) and never includes a token/context value
// (原则 3 — those are deferred to spec-3.8/3.10).
func TestStatusSegments_RichChrome(t *testing.T) {
	t.Parallel()
	m := New(fake.New(), Options{ModelName: "deepseek", Cwd: "~/opendbx", GitBranch: "main"})
	segs := m.StatusSegments()
	var texts []string
	for _, s := range segs {
		texts = append(texts, s.Text)
	}
	want := map[string]bool{"deepseek": false, "~/opendbx": false, "main": false}
	for _, txt := range texts {
		if _, ok := want[txt]; ok {
			want[txt] = true
		}
	}
	for k, seen := range want {
		if !seen {
			t.Errorf("status segments missing %q; got %v", k, texts)
		}
	}
}
