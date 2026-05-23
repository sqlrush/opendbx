// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File registry.go — thread-safe per-tool adapter registry (spec-1.9 D-2).
// Maps tool Name string → HeaderRenderer (required interface).
// Optional sub-interfaces (ProgressRenderer / QueuedRenderer) discovered
// via type assertion by ToolUse.Render at call time.

package adapter

import "sync"

// Registry is a thread-safe map of tool Name → HeaderRenderer. Register
// is typically called during init() of each tool's adapter package
// (e.g., adapter/bash/init.go). Lookup is called per-render by ToolUse.
//
// The zero value is a usable empty Registry; use NewRegistry for a
// named alternative if multiple registries are needed (none in
// spec-1.9 scope; spec-1.20 LLM client may host a separate test
// registry).
type Registry struct {
	mu       sync.RWMutex
	adapters map[string]HeaderRenderer
}

// NewRegistry returns an empty Registry. Equivalent to zero value but
// explicit for documentation.
func NewRegistry() *Registry {
	return &Registry{adapters: make(map[string]HeaderRenderer)}
}

// Register associates name → renderer in the registry. Subsequent
// Register calls with the same name overwrite (intentional: allows
// tests to inject mock adapters; production tools call once at init).
//
// renderer may also satisfy ProgressRenderer and/or QueuedRenderer;
// ToolUse.Render discovers via type assertion.
func (r *Registry) Register(name string, renderer HeaderRenderer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.adapters == nil {
		r.adapters = make(map[string]HeaderRenderer)
	}
	r.adapters[name] = renderer
}

// Lookup returns the HeaderRenderer for name, or nil if unregistered.
// Caller (ToolUse.Render) falls back to Generic adapter (D-5) when nil.
func (r *Registry) Lookup(name string) HeaderRenderer {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.adapters == nil {
		return nil
	}
	return r.adapters[name]
}

// Default is the package-level singleton Registry used by ToolUse.Render
// when no per-instance registry is injected. Tool adapter packages call
// adapter.Default.Register(name, &impl{}) in init() to wire themselves.
var Default = NewRegistry()
