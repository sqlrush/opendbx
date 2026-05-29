// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File registry.go — minimal in-memory ToolExecutor registry (spec-1.21
// D-3). Static / process-lifetime; dynamic skill loading + user-config
// driven registration is spec-2.1 skill-registry and intentionally NOT
// pre-built here (per user T-5 boundary: keep it small, no spec-2.1
// surface area leak).
//
// Invariants enforced at construction:
//   - tool names unique (duplicate → ErrRequestInvalid).
//   - Schema().Name == Name() for every executor (spec D-3 R-12 — the
//     name the LLM sees in the schema MUST equal the dispatch key).
//
// Lookup is read-only after New(); no Register/Unregister methods exist
// so a Registry value is safe for concurrent reads without locking.

package diagnose

import (
	"sort"

	"github.com/sqlrush/opendbx/internal/domain/llm"
)

// Registry maps tool Name → ToolExecutor. Constructed once and read-only.
type Registry struct {
	tools map[string]ToolExecutor
	names []string // sorted; deterministic Schemas() / Names() order
}

// NewRegistry builds a Registry from the provided executors. Returns
// LLM.REQUEST_INVALID if any executor has an empty name, a duplicate
// name, or a Schema().Name mismatching Name().
func NewRegistry(execs ...ToolExecutor) (*Registry, error) {
	r := &Registry{tools: make(map[string]ToolExecutor, len(execs))}
	for _, e := range execs {
		name := e.Name()
		if name == "" {
			// errcode-lint:exempt -- spec-1.21 D-3: RequestInvalidf returns LLM.REQUEST_INVALID errcode; pass-through.
			return nil, llm.RequestInvalidf("ToolExecutor with empty Name() in registry")
		}
		if e.Schema().Name != name {
			// errcode-lint:exempt -- spec-1.21 D-3: RequestInvalidf returns LLM.REQUEST_INVALID errcode; pass-through.
			return nil, llm.RequestInvalidf("ToolExecutor " + name + " has Schema().Name=" + e.Schema().Name + " (must equal Name())")
		}
		if _, dup := r.tools[name]; dup {
			// errcode-lint:exempt -- spec-1.21 D-3: RequestInvalidf returns LLM.REQUEST_INVALID errcode; pass-through.
			return nil, llm.RequestInvalidf("ToolExecutor " + name + " registered twice")
		}
		r.tools[name] = e
		r.names = append(r.names, name)
	}
	sort.Strings(r.names) // deterministic Schemas() / Names()
	return r, nil
}

// Get returns the executor for name, or false if unregistered. The Loop
// surfaces the latter as DIAGNOSE.TOOL_UNKNOWN (spec-1.21 D-4 / D-5).
func (r *Registry) Get(name string) (ToolExecutor, bool) {
	if r == nil {
		return nil, false
	}
	e, ok := r.tools[name]
	return e, ok
}

// Schemas returns all registered schemas in deterministic (name-sorted)
// order so Request.Tools is stable across runs — useful for prompt cache
// reuse (spec-1.20 cache_control) and for golden tests.
func (r *Registry) Schemas() []llm.ToolSchema {
	if r == nil {
		return nil
	}
	out := make([]llm.ToolSchema, 0, len(r.names))
	for _, n := range r.names {
		out = append(out, r.tools[n].Schema())
	}
	return out
}

// Names returns the sorted tool name list (mirrors Schemas() order).
func (r *Registry) Names() []string {
	if r == nil {
		return nil
	}
	out := make([]string, len(r.names))
	copy(out, r.names)
	return out
}

// Len reports the number of registered tools.
func (r *Registry) Len() int {
	if r == nil {
		return 0
	}
	return len(r.tools)
}
