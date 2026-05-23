// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package adapter

import (
	"strings"
	"sync"
	"testing"
)

func TestRegistry_RegisterLookup(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.Register("x", Generic{})
	if got := r.Lookup("x"); got == nil {
		t.Errorf("after Register: Lookup should return registered renderer")
	}
	if got := r.Lookup("missing"); got != nil {
		t.Errorf("unregistered Lookup should return nil, got %v", got)
	}
}

func TestRegistry_ZeroValueUsable(t *testing.T) {
	t.Parallel()
	var r Registry
	r.Register("x", Generic{})
	if got := r.Lookup("x"); got == nil {
		t.Errorf("zero-value Registry: Lookup should return registered renderer")
	}
}

func TestRegistry_ConcurrentWrites(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); r.Register("name", Bash{}) }()
		go func() { defer wg.Done(); _ = r.Lookup("name") }()
	}
	wg.Wait()
}

func TestBash_RenderHeader_Truncate(t *testing.T) {
	t.Parallel()
	b := Bash{}
	long := strings.Repeat("x", 200)
	out, _ := b.RenderHeader(map[string]any{"command": long}, Context{})
	// 160 chars truncated, with '…' rune (3 bytes UTF-8) appended; total ≤ 162 bytes.
	if len(out) > 162 {
		t.Errorf("non-verbose truncate failed: len=%d, want ≤162", len(out))
	}
	if !strings.HasSuffix(out, "…") {
		t.Errorf("non-verbose truncate should end with '…': %q", out)
	}
	outV, _ := b.RenderHeader(map[string]any{"command": long}, Context{Verbose: true})
	if len(outV) != 200 {
		t.Errorf("verbose should not truncate: len=%d", len(outV))
	}
}

func TestBash_RenderHeader_NoCommand(t *testing.T) {
	t.Parallel()
	b := Bash{}
	out, _ := b.RenderHeader(nil, Context{})
	if out != "(no command)" {
		t.Errorf("nil input: want '(no command)', got %q", out)
	}
}

func TestBash_RenderHeader_NonStringCommand(t *testing.T) {
	t.Parallel()
	b := Bash{}
	out, _ := b.RenderHeader(map[string]any{"command": 42}, Context{})
	if out != "(no command)" {
		t.Errorf("non-string command: want '(no command)', got %q", out)
	}
}

func TestBash_RenderProgress(t *testing.T) {
	t.Parallel()
	b := Bash{}
	out, _ := b.RenderProgress(nil, Context{})
	if out != "Running…" {
		t.Errorf("empty progress: got %q", out)
	}
	out2, _ := b.RenderProgress([]ProgressMessage{{ElapsedSeconds: 1, TotalLines: 5, TotalBytes: 100, TimeoutMs: 1000}}, Context{})
	for _, sub := range []string{"1s", "5 lines", "100B", "timeout 1000ms"} {
		if !strings.Contains(out2, sub) {
			t.Errorf("progress missing %q: %q", sub, out2)
		}
	}
}

func TestBash_RenderQueued(t *testing.T) {
	t.Parallel()
	b := Bash{}
	out, _ := b.RenderQueued()
	if out != "Waiting…" {
		t.Errorf("Queued: got %q", out)
	}
}

func TestRead_RenderHeader_Default(t *testing.T) {
	t.Parallel()
	r := Read{}
	out, _ := r.RenderHeader(map[string]any{"path": "main.go"}, Context{})
	if out != "main.go" {
		t.Errorf("default: got %q", out)
	}
}

func TestRead_RenderHeader_VerboseLines(t *testing.T) {
	t.Parallel()
	r := Read{}
	out, _ := r.RenderHeader(map[string]any{"path": "x.go", "offset": 10, "limit": 50}, Context{Verbose: true})
	if !strings.Contains(out, "lines 10-59") {
		t.Errorf("verbose lines: got %q", out)
	}
}

func TestRead_RenderHeader_OffsetOnly(t *testing.T) {
	t.Parallel()
	r := Read{}
	out, _ := r.RenderHeader(map[string]any{"path": "x.go", "offset": 100}, Context{Verbose: true})
	if !strings.Contains(out, "from line 100") {
		t.Errorf("offset only: got %q", out)
	}
}

func TestRead_RenderHeader_LimitOnly(t *testing.T) {
	t.Parallel()
	r := Read{}
	out, _ := r.RenderHeader(map[string]any{"path": "x.go", "limit": 20}, Context{Verbose: true})
	if !strings.Contains(out, "first 20 lines") {
		t.Errorf("limit only: got %q", out)
	}
}

func TestRead_RenderHeader_PagesPDF(t *testing.T) {
	t.Parallel()
	r := Read{}
	out, _ := r.RenderHeader(map[string]any{"path": "x.pdf", "pages": "1-3"}, Context{})
	if out != "x.pdf · pages 1-3" {
		t.Errorf("pages: got %q", out)
	}
}

func TestRead_RenderHeader_NoPath(t *testing.T) {
	t.Parallel()
	r := Read{}
	out, _ := r.RenderHeader(nil, Context{})
	if out != "(no path)" {
		t.Errorf("nil path: got %q", out)
	}
}

func TestRead_ParseStringOffsetLimit(t *testing.T) {
	t.Parallel()
	r := Read{}
	// LLM might send numbers as strings.
	out, _ := r.RenderHeader(map[string]any{"path": "x.go", "offset": "5", "limit": "10"}, Context{Verbose: true})
	if !strings.Contains(out, "lines 5-14") {
		t.Errorf("string offset/limit: got %q", out)
	}
}

func TestGeneric_RenderHeader_Compact(t *testing.T) {
	t.Parallel()
	g := Generic{}
	out, _ := g.RenderHeader(map[string]any{"_name": "T", "a": "1", "b": "2"}, Context{Cols: 80})
	if !strings.Contains(out, "T(") {
		t.Errorf("missing name: %q", out)
	}
	if !strings.Contains(out, "a=1") || !strings.Contains(out, "b=2") {
		t.Errorf("missing args: %q", out)
	}
}

func TestGeneric_NoName(t *testing.T) {
	t.Parallel()
	g := Generic{}
	out, _ := g.RenderHeader(map[string]any{"a": "1"}, Context{Cols: 80})
	if !strings.HasPrefix(out, "(unnamed)") {
		t.Errorf("no name: %q", out)
	}
}

func TestGeneric_TruncateBudget(t *testing.T) {
	t.Parallel()
	g := Generic{}
	out, _ := g.RenderHeader(map[string]any{"_name": "T", "k": strings.Repeat("v", 200)}, Context{Cols: 40})
	if !strings.HasSuffix(out, "…)") && !strings.HasSuffix(out, "…") {
		t.Errorf("budget truncate: got %q", out)
	}
}

func TestGeneric_SkipsSyntheticKeys(t *testing.T) {
	t.Parallel()
	g := Generic{}
	out, _ := g.RenderHeader(map[string]any{"_name": "T", "_meta": "x", "a": "1"}, Context{Cols: 80})
	if strings.Contains(out, "_meta") {
		t.Errorf("should skip _meta synthetic key: %q", out)
	}
}
