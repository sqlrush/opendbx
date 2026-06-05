// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File errors.go — SKILL.* invoke-time errcode sentinels (spec-2.3 D-6;
// 规则 7 三件套). Both codes are FEEDBACK class: their composed templates
// are written into ToolOutput.Content with IsError=true so the model
// self-corrects on the next turn — they never terminate the Run.
//
// The third spec-2.3 code, SKILL.SCOPE_TOOL_DENIED, is registered in
// app/diagnose/errors.go: its text is composed by the Loop dispatch
// guard (loop.go), and diagnose cannot import this package (invoke →
// diagnose would become a cycle). Same placement precedent as
// DIAGNOSE.TOOL_TIMEOUT (feedback text composed by the Loop).

package invoke

import (
	"fmt"
	"strings"

	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

//nolint:gochecknoglobals // spec-0.6 contract: errcode sentinels are package-level.
var (
	// ErrNotFound — the requested skill name is not in the active set.
	// Feedback class (recoverable; the composed Content lists the active
	// skills so the model can self-correct).
	ErrNotFound = errcode.Register(
		"SKILL.NOT_FOUND",
		"no active skill with the requested name",
		"check /debug skills for the active list",
	)
	// ErrInvokeInvalid — the Skill tool input shape is invalid (missing /
	// non-string "skill", or non-object "args"). Feedback class.
	ErrInvokeInvalid = errcode.Register(
		"SKILL.INVOKE_INVALID",
		"Skill tool input is invalid",
		`call Skill with {"skill": "<name>"} and optional object "args"`,
	)
)

// notFoundContent composes the recoverable ToolOutput.Content for an
// unknown skill name (spec-2.3 D-6 template). names must be sorted for
// deterministic output (SkillTool guarantees this).
func notFoundContent(name string, names []string) string {
	return fmt.Sprintf("%s: no active skill named %q. Available skills: [%s]. Hint: %s.",
		ErrNotFound.Code(), name, strings.Join(names, ", "), ErrNotFound.Hint())
}

// invokeInvalidContent composes the recoverable ToolOutput.Content for a
// malformed Skill tool input (spec-2.3 D-6 template). detail is one of
// the fixed-detail variants pinned by the spec (missing/non-string skill,
// non-object args).
func invokeInvalidContent(detail string) string {
	return fmt.Sprintf("%s: %s. Hint: %s.",
		ErrInvokeInvalid.Code(), detail, ErrInvokeInvalid.Hint())
}
