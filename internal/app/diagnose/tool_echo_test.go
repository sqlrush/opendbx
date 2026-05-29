// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package diagnose

import (
	"context"
	"encoding/json"
	"testing"
)

// TestEchoTool_RoundTripsInput confirms the model sees its input verbatim
// in the next turn's tool_result — the headline T-5 end-to-end demo.
func TestEchoTool_RoundTripsInput(t *testing.T) {
	t.Parallel()
	in := map[string]any{"message": "hi", "n": float64(3)}
	out, err := EchoTool{}.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if out.IsError {
		t.Errorf("echo should never IsError on a happy input")
	}
	var got map[string]any
	if e := json.Unmarshal([]byte(out.Content), &got); e != nil {
		t.Fatalf("Content not JSON: %q err=%v", out.Content, e)
	}
	if got["message"] != "hi" || got["n"] != float64(3) {
		t.Errorf("round-trip lost fields: got=%v", got)
	}
}

// TestEchoTool_NilInput: nil map round-trips as "{}" (matches the
// OpenAI tool-arg shape; safe canonical empty form).
func TestEchoTool_NilInput(t *testing.T) {
	t.Parallel()
	out, err := EchoTool{}.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if out.Content != "{}" || out.IsError {
		t.Errorf("nil input → %+v; want Content=%q IsError=false", out, "{}")
	}
}

// TestEchoTool_RespectsCancelledCtx mirrors the clock guard. Echo MUST
// honor ctx.Err so Loop sees a clean cancel signal during long-running
// tool sequences (D-4 cancel-vs-timeout three-way dispatch).
func TestEchoTool_RespectsCancelledCtx(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := EchoTool{}.Execute(ctx, map[string]any{"x": 1})
	if err == nil {
		t.Errorf("expected ctx.Err; got nil")
	}
}

// TestEchoTool_NoSideEffects pins the scope discipline of spec-1.21 T-5:
// echo's Schema MUST describe pure input return, so a code reader (or
// the LLM) cannot mistake it for a shell/SQL bridge. This is a doc-
// level guard but it costs nothing to enforce here.
func TestEchoTool_NoSideEffects(t *testing.T) {
	t.Parallel()
	s := EchoTool{}.Schema()
	if s.Name != "echo" {
		t.Errorf("Name = %q; want echo", s.Name)
	}
	for _, banned := range []string{"shell", "exec", "sql", "filesystem", "fs", "command"} {
		if containsCI(s.Description, banned) {
			t.Errorf("echo description must not promise %q (T-5 scope discipline); got %q", banned, s.Description)
		}
	}
}

func containsCI(haystack, needle string) bool {
	if len(needle) == 0 {
		return false
	}
	// Cheap ASCII case-insensitive contains; avoids a strings import
	// re-pulled into this very small test file.
	hb := []byte(haystack)
	nb := []byte(needle)
	for i := range hb {
		if i+len(nb) > len(hb) {
			return false
		}
		match := true
		for j := range nb {
			a, b := hb[i+j], nb[j]
			if a >= 'A' && a <= 'Z' {
				a += 'a' - 'A'
			}
			if b >= 'A' && b <= 'Z' {
				b += 'a' - 'A'
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
