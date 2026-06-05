// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File skilltool.go — SkillTool, the diagnose.ToolExecutor that lets the
// model enter a skill scope (spec-2.3 D-1).
//
// CC baseline: src/tools/SkillTool/ (survey skills.md § 1.5) — input
// {skill, args}; the skill body enters the model context and
// allowed-tools is enforced for the scope. Two deliberate deviations,
// both pinned by spec-2.3:
//   - body is returned as the tool RESULT (tool_result block), not
//     injected into the system prompt (Q11, user-approved divergence
//     from the survey § 2.3 wording; revisit on T-11 verify).
//   - "Skill" itself is implicitly retained by the Loop scope filter
//     (Q13 meta-control router), so allowed-tools never needs to list it.
//
// Failure two-track (spec-1.21 tool.go contract):
//   - semantic failures (unknown skill, bad input shape) → recoverable
//     ToolOutput{IsError: true} — the model self-corrects next turn.
//   - infrastructure failures (ctx cancel / deadline) → Go error, fatal;
//     the Loop's classifyToolErr does the cancel-vs-timeout dispatch.

package invoke

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/sqlrush/opendbx/internal/app/diagnose"
	"github.com/sqlrush/opendbx/internal/app/skills"
	"github.com/sqlrush/opendbx/internal/domain/llm"
	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

// ToolName is the wire-visible tool identifier — capitalized for 1:1 CC
// parity (spec-2.3 Q3; divergence from lowercase clock/echo is deliberate).
const ToolName = "Skill"

// modelNoticeFmt is appended to the invoke Content when the skill's
// frontmatter requests a model override (spec-2.3 Q7: v1 never switches
// models; visible notice instead of silent ignore, 规则 7).
const modelNoticeFmt = "\n\nNote: this skill requests model %q; per-skill model switching lands in spec-3.11."

// SkillTool maps the validated active skill set (spec-2.2
// DiscoveryResult.Active) onto the diagnose loop. Immutable after
// construction: Execute never mutates internal state, so the process-wide
// registry sharing across Runs is safe (spec-2.3 R-6 — scope state lives
// in Loop.Run locals, never here).
type SkillTool struct {
	byKey map[string]skills.Skill // Key() → Skill (case-sensitive, 2.1 D-2)
	names []string                // sorted; deterministic listings
}

// NewSkillTool builds the tool from the active skill set. It defensively
// asserts the spec-2.2 Active membership contract (unique Key per skill);
// a violation is an upstream bug and returns an error — the bootstrap
// caller LOGS and SKIPS registration, it never panics (bad skills must
// not block interact; spec-2.3 user decision 4/4 2026-06-05).
func NewSkillTool(active []skills.Skill) (*SkillTool, error) {
	if len(active) == 0 {
		return nil, errcode.Newf("SKILL.INVOKE_INVALID",
			"NewSkillTool requires a non-empty active skill set; skip construction when discovery yields none")
	}
	byKey := make(map[string]skills.Skill, len(active))
	names := make([]string, 0, len(active))
	for _, sk := range active {
		key := sk.Key()
		if _, dup := byKey[key]; dup {
			return nil, errcode.Newf("SKILL.NAMESPACE_CONFLICT",
				"duplicate active skill key %q passed to NewSkillTool (spec-2.2 Active membership contract violated)", key)
		}
		byKey[key] = sk
		names = append(names, key)
	}
	sort.Strings(names)
	return &SkillTool{byKey: byKey, names: names}, nil
}

// Name implements diagnose.ToolExecutor.
func (t *SkillTool) Name() string { return ToolName }

// Schema implements diagnose.ToolExecutor. Input mirrors the CC SkillTool
// contract {skill, args} (survey § 1.5; line-level provisional, T-11 #3).
// Unknown extra input keys are ignored at Execute (tolerant decode).
func (t *SkillTool) Schema() llm.ToolSchema {
	return llm.ToolSchema{
		Name: ToolName,
		Description: "Invoke an installed skill by exact name. The skill's instructions are " +
			"returned for you to follow. Use when an available skill (listed in the system " +
			"prompt) matches the current task; do not guess names not on that list.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"skill": map[string]any{
					"type":        "string",
					"description": "Exact name of an available skill (case-sensitive).",
				},
				"args": map[string]any{
					"type":                 "object",
					"description":          "Optional arguments for the skill.",
					"additionalProperties": true,
				},
			},
			"required": []string{"skill"},
		},
	}
}

// Execute implements diagnose.ToolExecutor (spec-2.3 D-1 steps 1-6).
func (t *SkillTool) Execute(ctx context.Context, input map[string]any) (diagnose.ToolOutput, error) {
	// errcode-lint:exempt -- spec-1.21 D-4 two-track: ctx errors pass through unchanged; the Loop classifies cancel-vs-timeout via errors.Is.
	if err := ctx.Err(); err != nil {
		return diagnose.ToolOutput{}, err
	}
	name, ok := input["skill"].(string)
	if !ok || strings.TrimSpace(name) == "" {
		return diagnose.ToolOutput{
			Content: invokeInvalidContent(`missing or non-string "skill" field`),
			IsError: true,
		}, nil
	}
	argsJSON, argsErr := canonicalArgs(input)
	if argsErr != "" {
		return diagnose.ToolOutput{Content: invokeInvalidContent(argsErr), IsError: true}, nil
	}
	sk, found := t.byKey[name] // case-sensitive (2.1 D-2: Key is byte-exact)
	if !found {
		return diagnose.ToolOutput{Content: notFoundContent(name, t.names), IsError: true}, nil
	}

	var b strings.Builder
	b.WriteString(sk.Body) // byte-verbatim (2.1 body fidelity; golden-tested)
	if argsJSON != "" {
		b.WriteString("\n\n## Arguments\n```json\n")
		b.WriteString(argsJSON)
		b.WriteString("\n```")
	}
	if m := sk.Schema.Model; m != "" {
		fmt.Fprintf(&b, modelNoticeFmt, m)
	}
	return diagnose.ToolOutput{
		Content:    b.String(),
		ToolFilter: sk.Schema.AllowedToolsList(), // nil = inherit scope (2.1: ""→nil)
	}, nil
}

// canonicalArgs validates and renders the optional "args" input
// (spec-2.3 D-1 step 3 five-shape table):
//
//	absent        → ("", "")   — no Arguments section
//	object        → (canonical key-sorted JSON, "")
//	null / string / list / number / bool → ("", detail) — recoverable
//
// json.Marshal sorts map keys (stdlib), and DecodeToolInput delivers
// numbers as json.Number, so the rendering is deterministic across turns
// (this also keeps the spec-1.22 dedup key stable for identical args).
func canonicalArgs(input map[string]any) (argsJSON, errDetail string) {
	v, present := input["args"]
	if !present {
		return "", ""
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return "", fmt.Sprintf(`"args" must be a JSON object, got %s`, jsonTypeName(v))
	}
	raw, err := json.Marshal(obj)
	if err != nil {
		// Unreachable under the DecodeToolInput invariant (JSON-decodable
		// values only); reported recoverably rather than swallowed.
		return "", `"args" could not be re-encoded as JSON: ` + err.Error()
	}
	return string(raw), ""
}

// jsonTypeName names a decoded JSON value's type for error messages
// (model-facing wording, hence JSON terms — not Go type syntax).
func jsonTypeName(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case []any:
		return "array"
	case bool:
		return "boolean"
	case json.Number, float64, int, int64:
		return "number"
	default:
		return fmt.Sprintf("%T", v)
	}
}
