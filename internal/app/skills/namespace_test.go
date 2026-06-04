// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package skills

import (
	"reflect"
	"testing"
)

func sk(name string, prec int) Skill {
	return Skill{
		Schema: Schema{Name: name, Description: "d"},
		Source: SkillSource{Precedence: prec, Path: name},
	}
}

func TestResolve_NoConflict(t *testing.T) {
	t.Parallel()
	res := Resolve([]Skill{sk("a", 1), sk("b", 1)})
	if len(res.Active) != 2 || len(res.Conflicts) != 0 || len(res.Shadowed) != 0 {
		t.Fatalf("unexpected resolution: %+v", res)
	}
	// Sorted by name.
	if res.Active[0].Schema.Name != "a" || res.Active[1].Schema.Name != "b" {
		t.Errorf("Active not sorted by name: %+v", res.Active)
	}
}

func TestResolve_ShadowByPrecedence(t *testing.T) {
	t.Parallel()
	low := sk("top-sql", 1)
	high := sk("top-sql", 10)
	res := Resolve([]Skill{low, high})
	if len(res.Active) != 1 || res.Active[0].Source.Precedence != 10 {
		t.Fatalf("higher precedence should win: %+v", res.Active)
	}
	if len(res.Shadowed) != 1 || res.Shadowed[0].Source.Precedence != 1 {
		t.Fatalf("lower precedence should be shadowed: %+v", res.Shadowed)
	}
	if len(res.Conflicts) != 1 || res.Conflicts[0].Kind != ConflictShadowed {
		t.Fatalf("expected one ConflictShadowed: %+v", res.Conflicts)
	}
	c := res.Conflicts[0]
	if c.Winner == nil || c.Winner.Source.Precedence != 10 {
		t.Errorf("conflict winner wrong: %+v", c)
	}
}

func TestResolve_Unresolvable(t *testing.T) {
	t.Parallel()
	res := Resolve([]Skill{sk("dup", 5), sk("dup", 5)})
	if len(res.Active) != 0 {
		t.Fatalf("unresolvable must not pick a winner: %+v", res.Active)
	}
	if len(res.Conflicts) != 1 || res.Conflicts[0].Kind != ConflictUnresolvable {
		t.Fatalf("expected ConflictUnresolvable: %+v", res.Conflicts)
	}
	if res.Conflicts[0].Winner != nil {
		t.Errorf("unresolvable winner must be nil: %+v", res.Conflicts[0])
	}
	if len(res.Conflicts[0].Shadowed) != 2 {
		t.Errorf("unresolvable should list all duplicates: %+v", res.Conflicts[0])
	}
}

func TestResolve_EmptyNameSegregated(t *testing.T) {
	t.Parallel()
	// Two empty-name skills must NOT collapse into one false conflict.
	res := Resolve([]Skill{{Schema: Schema{Name: ""}}, {Schema: Schema{Name: ""}}, sk("real", 1)})
	if len(res.Conflicts) != 0 {
		t.Errorf("empty names must not create a conflict: %+v", res.Conflicts)
	}
	if len(res.Active) != 1 || res.Active[0].Schema.Name != "real" {
		t.Errorf("only the named skill should be active: %+v", res.Active)
	}
}

func TestDetectConflicts(t *testing.T) {
	t.Parallel()
	got := DetectConflicts([]Skill{sk("x", 1), sk("x", 2)})
	if len(got) != 1 || got[0].Kind != ConflictShadowed {
		t.Errorf("DetectConflicts = %+v", got)
	}
}

// TestResolve_DoesNotMutateInput — review LOW-2: Resolve must not mutate the
// input slice or the Skills' Extra maps. Deep-compare a snapshot.
func TestResolve_DoesNotMutateInput(t *testing.T) {
	t.Parallel()
	input := []Skill{
		{Schema: Schema{Name: "a", Description: "d", Extra: map[string]any{"k": 1}}, Source: SkillSource{Precedence: 2}},
		{Schema: Schema{Name: "a", Description: "d", Extra: map[string]any{"k": 2}}, Source: SkillSource{Precedence: 1}},
		{Schema: Schema{Name: "b", Description: "d"}, Source: SkillSource{Precedence: 3}},
	}
	snapshot := deepCopySkills(input)
	_ = Resolve(input)
	if !reflect.DeepEqual(input, snapshot) {
		t.Errorf("Resolve mutated its input:\n got  %+v\n want %+v", input, snapshot)
	}
}

// TestResolve_OutputIsolatedFromInput — review HIGH: mutating a returned
// Skill's Extra map must NOT reach back into the input slice.
func TestResolve_OutputIsolatedFromInput(t *testing.T) {
	t.Parallel()
	input := []Skill{
		{Schema: Schema{Name: "a", Description: "d", Extra: map[string]any{"k": map[string]any{"deep": 1}}}, Source: SkillSource{Precedence: 2}},
		{Schema: Schema{Name: "a", Description: "d", Extra: map[string]any{"k": 9}}, Source: SkillSource{Precedence: 1}},
	}
	res := Resolve(input)
	// Mutate the winner's Extra (top-level + nested) through the output.
	res.Active[0].Schema.Extra["injected"] = 999
	if nested, ok := res.Active[0].Schema.Extra["k"].(map[string]any); ok {
		nested["deep"] = 42
	}
	// Mutate a shadowed copy too.
	if len(res.Shadowed) > 0 {
		res.Shadowed[0].Schema.Extra["injected2"] = 7
	}
	if _, leaked := input[0].Schema.Extra["injected"]; leaked {
		t.Error("output mutation leaked into input[0].Extra (top-level)")
	}
	if nested, ok := input[0].Schema.Extra["k"].(map[string]any); ok {
		if nested["deep"] != 1 {
			t.Errorf("output mutation leaked into input nested Extra: %v", nested["deep"])
		}
	}
	if _, leaked := input[1].Schema.Extra["injected2"]; leaked {
		t.Error("shadowed mutation leaked into input[1].Extra")
	}
}

func TestConflict_Err(t *testing.T) {
	t.Parallel()
	res := Resolve([]Skill{sk("dup", 5), sk("dup", 5)})
	if len(res.Conflicts) != 1 {
		t.Fatalf("want 1 conflict, got %d", len(res.Conflicts))
	}
	err := res.Conflicts[0].Err()
	if err == nil {
		t.Fatal("unresolvable conflict must yield an error")
	}
	var ec interface{ Code() string }
	if !asCode(err, &ec) || ec.Code() != ErrNamespaceConflict.Code() {
		t.Errorf("Conflict.Err code = %v; want %s", err, ErrNamespaceConflict.Code())
	}
	// A resolved shadow yields no error.
	shadow := Resolve([]Skill{sk("x", 1), sk("x", 2)})
	if shadow.Conflicts[0].Err() != nil {
		t.Errorf("shadowed conflict must not error: %v", shadow.Conflicts[0].Err())
	}
}

func asCode(err error, target *interface{ Code() string }) bool {
	if c, ok := err.(interface{ Code() string }); ok {
		*target = c
		return true
	}
	return false
}

func deepCopySkills(in []Skill) []Skill {
	out := make([]Skill, len(in))
	for i, s := range in {
		out[i] = s
		if s.Schema.Extra != nil {
			cp := make(map[string]any, len(s.Schema.Extra))
			for k, v := range s.Schema.Extra {
				cp[k] = v
			}
			out[i].Schema.Extra = cp
		}
	}
	return out
}
