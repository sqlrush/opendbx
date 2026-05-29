// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package diagnose

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/sqlrush/opendbx/internal/domain/llm"
)

// nameStub is a minimal ToolExecutor for registry-shape tests.
type nameStub struct {
	name       string
	schemaName string // override; "" → mirror name (consistent case)
}

func (s nameStub) Name() string { return s.name }
func (s nameStub) Schema() llm.ToolSchema {
	sn := s.schemaName
	if sn == "" {
		sn = s.name
	}
	return llm.ToolSchema{Name: sn, InputSchema: map[string]any{"type": "object"}}
}
func (nameStub) Execute(context.Context, map[string]any) (ToolOutput, error) {
	return ToolOutput{Content: "stub"}, nil
}

// TestNewRegistry_AcceptsBuiltins is the headline T-5 case: the clock +
// echo bundle the loop ships with builds cleanly and Schemas() returns
// the two schemas in deterministic order.
func TestNewRegistry_AcceptsBuiltins(t *testing.T) {
	t.Parallel()
	r, err := NewRegistry(ClockTool{}, EchoTool{})
	if err != nil {
		t.Fatalf("NewRegistry err: %v", err)
	}
	if r.Len() != 2 {
		t.Errorf("Len = %d; want 2", r.Len())
	}
	got := r.Names()
	if !reflect.DeepEqual(got, []string{"clock", "echo"}) {
		t.Errorf("Names = %v; want [clock echo] sorted", got)
	}
	schemas := r.Schemas()
	if len(schemas) != 2 || schemas[0].Name != "clock" || schemas[1].Name != "echo" {
		t.Errorf("Schemas order = %+v; want [clock echo]", schemas)
	}
}

// TestNewRegistry_RejectsDuplicateName: register the same name twice →
// LLM.REQUEST_INVALID, no silent overwrite.
func TestNewRegistry_RejectsDuplicateName(t *testing.T) {
	t.Parallel()
	_, err := NewRegistry(nameStub{name: "dup"}, nameStub{name: "dup"})
	if !errors.Is(err, llm.ErrRequestInvalid) {
		t.Errorf("duplicate → %v; want ErrRequestInvalid", err)
	}
}

// TestNewRegistry_RejectsEmptyName guards against accidentally registering
// the zero value of a struct executor (defensive — pointer types).
func TestNewRegistry_RejectsEmptyName(t *testing.T) {
	t.Parallel()
	_, err := NewRegistry(nameStub{name: ""})
	if !errors.Is(err, llm.ErrRequestInvalid) {
		t.Errorf("empty name → %v; want ErrRequestInvalid", err)
	}
}

// TestNewRegistry_RejectsSchemaNameMismatch is the spec D-3 R-12
// schema/registry consistency contract: the Name shown in the schema
// (what the LLM sees) MUST equal the dispatch key (what Loop uses).
func TestNewRegistry_RejectsSchemaNameMismatch(t *testing.T) {
	t.Parallel()
	_, err := NewRegistry(nameStub{name: "clock", schemaName: "klok"})
	if !errors.Is(err, llm.ErrRequestInvalid) {
		t.Errorf("mismatch → %v; want ErrRequestInvalid", err)
	}
}

// TestRegistry_Get verifies hit / miss semantics.
func TestRegistry_Get(t *testing.T) {
	t.Parallel()
	r, _ := NewRegistry(ClockTool{}, EchoTool{})
	if e, ok := r.Get("clock"); !ok || e.Name() != "clock" {
		t.Errorf("Get(clock) = %v ok=%v; want clock,true", e, ok)
	}
	if _, ok := r.Get("topsql"); ok {
		t.Errorf("Get(topsql) should miss")
	}
}

// TestRegistry_NilSafe verifies nil-receiver methods don't panic — Loop
// may carry a nil Registry when configured with no tools (no-tool single
// turn 退化路径).
func TestRegistry_NilSafe(t *testing.T) {
	t.Parallel()
	var r *Registry
	if _, ok := r.Get("anything"); ok {
		t.Errorf("nil Get should miss")
	}
	if r.Schemas() != nil || r.Names() != nil {
		t.Errorf("nil Schemas/Names should return nil")
	}
	if r.Len() != 0 {
		t.Errorf("nil Len = %d; want 0", r.Len())
	}
}

// TestRegistry_BuiltinSchemaConsistency is the cross-builtin variant of
// the R-12 invariant: every shipped tool's Schema().Name MUST equal
// Name() (table-driven so future additions get free coverage).
func TestRegistry_BuiltinSchemaConsistency(t *testing.T) {
	t.Parallel()
	for _, e := range []ToolExecutor{ClockTool{}, EchoTool{}} {
		if e.Schema().Name != e.Name() {
			t.Errorf("%s: Schema().Name=%q != Name()=%q", e.Name(), e.Schema().Name, e.Name())
		}
	}
}
