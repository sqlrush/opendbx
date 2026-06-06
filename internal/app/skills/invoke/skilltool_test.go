// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package invoke

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/sqlrush/opendbx/internal/app/skills"
	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

// mkSkill builds a minimal valid Skill for invoke tests.
func mkSkill(name, body string) skills.Skill {
	return skills.Skill{
		Schema: skills.Schema{Name: name, Description: "Test skill."},
		Body:   body,
	}
}

// loadGolden parses the CC golden skill (spec-2.1 D-7 corpus) so the
// invoke path is exercised against a byte-faithful real skill.
func loadGolden(t *testing.T) skills.Skill {
	t.Helper()
	raw, err := os.ReadFile("../testdata/code-reviewer.md")
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	sk, err := skills.Parse(raw, skills.SkillSource{Kind: skills.SourceProject, Precedence: 400})
	if err != nil {
		t.Fatalf("parse golden: %v", err)
	}
	return sk
}

// TestNewSkillTool_Validation — empty input and duplicate Key() are
// upstream contract violations and must surface as errors (the bootstrap
// caller logs + skips; spec-2.3 user decision 4/4 — never panic).
func TestNewSkillTool_Validation(t *testing.T) {
	t.Parallel()
	if _, err := NewSkillTool(nil); err == nil {
		t.Error("NewSkillTool(nil) = nil error, want error")
	}
	_, err := NewSkillTool([]skills.Skill{mkSkill("dup", "a"), mkSkill("dup", "b")})
	if err == nil {
		t.Fatal("NewSkillTool(dup keys) = nil error, want error")
	}
	var ec errcode.Error
	if !errors.As(err, &ec) || ec.Code() != "SKILL.NAMESPACE_CONFLICT" {
		t.Errorf("dup-key error code = %v, want SKILL.NAMESPACE_CONFLICT", err)
	}
}

// TestSkillTool_NameAndSchema — wire name is the CC-parity "Skill" and
// the schema requires the skill field (spec-2.3 Q3 + D-1).
func TestSkillTool_NameAndSchema(t *testing.T) {
	t.Parallel()
	st, err := NewSkillTool([]skills.Skill{mkSkill("alpha", "body")})
	if err != nil {
		t.Fatal(err)
	}
	if st.Name() != "Skill" {
		t.Errorf("Name() = %q, want Skill", st.Name())
	}
	sch := st.Schema()
	if sch.Name != st.Name() {
		t.Errorf("Schema().Name = %q must equal Name() (registry invariant)", sch.Name)
	}
	req, _ := sch.InputSchema["required"].([]string)
	if len(req) != 1 || req[0] != "skill" {
		t.Errorf("required = %v, want [skill]", req)
	}
}

// TestExecute_CtxFatal — a cancelled context is an infrastructure
// failure: Go error return, NOT a recoverable IsError (spec-2.3 R2
// two-track; FROZEN tool.go contract).
func TestExecute_CtxFatal(t *testing.T) {
	t.Parallel()
	st, _ := NewSkillTool([]skills.Skill{mkSkill("alpha", "body")})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out, err := st.Execute(ctx, map[string]any{"skill": "alpha"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute(cancelled ctx) err = %v, want context.Canceled", err)
	}
	if out.IsError || out.Content != "" {
		t.Errorf("cancelled ctx must not produce a recoverable output, got %+v", out)
	}
}

// TestExecute_InputValidation — semantic input failures are recoverable
// IsError with the INVOKE_INVALID / NOT_FOUND templates; the args field
// is validated across all five JSON shapes (spec-2.3 D-1 step 3).
func TestExecute_InputValidation(t *testing.T) {
	t.Parallel()
	st, _ := NewSkillTool([]skills.Skill{mkSkill("alpha", "body"), mkSkill("Beta", "b2")})
	tests := []struct {
		name     string
		input    map[string]any
		wantPart string
	}{
		{"missing skill", map[string]any{}, "SKILL.INVOKE_INVALID"},
		{"non-string skill", map[string]any{"skill": 42}, "SKILL.INVOKE_INVALID"},
		{"blank skill", map[string]any{"skill": "  "}, "SKILL.INVOKE_INVALID"},
		{"unknown skill", map[string]any{"skill": "nope"}, `SKILL.NOT_FOUND: no active skill named "nope". Available skills: [Beta, alpha]`},
		{"case mismatch is unknown", map[string]any{"skill": "beta"}, "SKILL.NOT_FOUND"},
		{"args null", map[string]any{"skill": "alpha", "args": nil}, `"args" must be a JSON object, got null`},
		{"args string", map[string]any{"skill": "alpha", "args": "s"}, `"args" must be a JSON object, got string`},
		{"args array", map[string]any{"skill": "alpha", "args": []any{1}}, `"args" must be a JSON object, got array`},
		{"args number", map[string]any{"skill": "alpha", "args": float64(3)}, `"args" must be a JSON object, got number`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, err := st.Execute(context.Background(), tc.input)
			if err != nil {
				t.Fatalf("semantic failure must not return Go error, got %v", err)
			}
			if !out.IsError {
				t.Fatalf("want IsError=true, got %+v", out)
			}
			if !strings.Contains(out.Content, tc.wantPart) {
				t.Errorf("Content %q missing %q", out.Content, tc.wantPart)
			}
			if out.ToolFilter != nil {
				t.Errorf("failure output must not carry ToolFilter, got %v", out.ToolFilter)
			}
		})
	}
}

// TestExecute_GoldenBody — invoking the CC golden skill returns the body
// byte-verbatim and passes allowed-tools through as the ToolFilter
// (spec-2.3 D-1; body fidelity inherited from the 2.1 golden contract).
func TestExecute_GoldenBody(t *testing.T) {
	t.Parallel()
	golden := loadGolden(t)
	st, err := NewSkillTool([]skills.Skill{golden})
	if err != nil {
		t.Fatal(err)
	}
	out, err := st.Execute(context.Background(), map[string]any{"skill": "code-reviewer"})
	if err != nil || out.IsError {
		t.Fatalf("golden invoke failed: err=%v out=%+v", err, out)
	}
	// Body byte-verbatim, then the model notice (golden has model: sonnet).
	if !strings.HasPrefix(out.Content, golden.Body) {
		t.Errorf("Content does not start with body bytes:\n%q", out.Content)
	}
	if !strings.Contains(out.Content, `requests model "sonnet"`) {
		t.Errorf("missing model notice (Q7):\n%q", out.Content)
	}
	wantFilter := []string{"Read", "Grep", "Glob", "Bash"}
	if !reflect.DeepEqual(out.ToolFilter, wantFilter) {
		t.Errorf("ToolFilter = %v, want %v", out.ToolFilter, wantFilter)
	}
}

// TestExecute_ArgsRendering — present-object args render as a canonical
// key-sorted JSON section after the body; absent args add no section.
func TestExecute_ArgsRendering(t *testing.T) {
	t.Parallel()
	st, _ := NewSkillTool([]skills.Skill{mkSkill("alpha", "BODY")})

	out, err := st.Execute(context.Background(), map[string]any{
		"skill": "alpha",
		"args":  map[string]any{"zeta": "1", "alpha": "2"},
	})
	if err != nil || out.IsError {
		t.Fatalf("invoke failed: %v %+v", err, out)
	}
	want := "BODY\n\n## Arguments\n```json\n{\"alpha\":\"2\",\"zeta\":\"1\"}\n```"
	if out.Content != want {
		t.Errorf("Content = %q, want %q (canonical key-sorted)", out.Content, want)
	}

	out2, _ := st.Execute(context.Background(), map[string]any{"skill": "alpha"})
	if out2.Content != "BODY" {
		t.Errorf("absent args must add no section, got %q", out2.Content)
	}
	// Empty object is present → renders an empty Arguments section.
	out3, _ := st.Execute(context.Background(), map[string]any{"skill": "alpha", "args": map[string]any{}})
	if !strings.Contains(out3.Content, "## Arguments\n```json\n{}\n```") {
		t.Errorf("empty-object args should render {}, got %q", out3.Content)
	}
}

// TestExecute_NoFilterWhenNoAllowedTools — a skill without allowed-tools
// yields ToolFilter nil (inherit scope; 2.1 ""→nil contract).
func TestExecute_NoFilterWhenNoAllowedTools(t *testing.T) {
	t.Parallel()
	st, _ := NewSkillTool([]skills.Skill{mkSkill("alpha", "b")})
	out, err := st.Execute(context.Background(), map[string]any{"skill": "alpha"})
	if err != nil || out.IsError {
		t.Fatalf("invoke failed: %v %+v", err, out)
	}
	if out.ToolFilter != nil {
		t.Errorf("ToolFilter = %v, want nil (inherit)", out.ToolFilter)
	}
}

// TestSkillTool_Immutable — Execute never mutates the tool's internal
// state (Rule 12 / spec-2.3 R-6: process-wide registry sharing).
func TestSkillTool_Immutable(t *testing.T) {
	t.Parallel()
	active := []skills.Skill{loadGolden(t), mkSkill("alpha", "b")}
	st, _ := NewSkillTool(active)
	namesBefore := append([]string{}, st.names...)
	for _, in := range []map[string]any{
		{"skill": "alpha"},
		{"skill": "nope"},
		{},
		{"skill": "code-reviewer", "args": map[string]any{"k": "v"}},
	} {
		if _, err := st.Execute(context.Background(), in); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(namesBefore, st.names) || len(st.byKey) != 2 {
		t.Error("SkillTool internal state mutated by Execute")
	}
}
