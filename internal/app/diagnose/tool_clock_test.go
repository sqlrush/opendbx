// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package diagnose

import (
	"context"
	"testing"
	"time"
)

// TestClockTool_FixedNow injects a known timestamp so the formatted
// output is fully deterministic — no hidden time.Now dependence.
func TestClockTool_FixedNow(t *testing.T) {
	t.Parallel()
	fixed := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	tool := ClockTool{Now: func() time.Time { return fixed }}
	out, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if out.IsError {
		t.Errorf("IsError should be false on success")
	}
	if out.Content != "2026-05-29T12:00:00Z" {
		t.Errorf("Content = %q; want 2026-05-29T12:00:00Z (RFC3339 UTC)", out.Content)
	}
}

// TestClockTool_DefaultNow exercises the nil-Now → time.Now fallback so
// callers using the zero value of ClockTool still get a current
// timestamp. We accept any UTC RFC3339 string here.
func TestClockTool_DefaultNow(t *testing.T) {
	t.Parallel()
	out, err := ClockTool{}.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if _, perr := time.Parse(time.RFC3339, out.Content); perr != nil {
		t.Errorf("Content %q not parseable as RFC3339: %v", out.Content, perr)
	}
}

// TestClockTool_RespectsCancelledCtx covers the per-tool ctx deadline
// contract (spec-1.21 D-3): cancelled ctx → ctx.Err returned before
// producing output, so Loop's cancel-vs-timeout dispatch can classify
// the failure correctly (D-4).
func TestClockTool_RespectsCancelledCtx(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ClockTool{}.Execute(ctx, nil)
	if err == nil {
		t.Errorf("expected ctx.Err; got nil")
	}
}

// TestClockTool_SchemaShape pins the JSON-schema-shaped Schema() output
// so OpenAI-compat providers requiring Parameters do not 400 on tool
// injection — empty properties, type=object.
func TestClockTool_SchemaShape(t *testing.T) {
	t.Parallel()
	s := ClockTool{}.Schema()
	if s.Name != "clock" || s.Description == "" {
		t.Errorf("Schema = %+v; want name=clock + non-empty description", s)
	}
	if tp, _ := s.InputSchema["type"].(string); tp != "object" {
		t.Errorf("InputSchema type = %v; want object", s.InputSchema["type"])
	}
}
