// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File errors.go — SKILL.* errcode sentinels (spec-2.1 D-6; 规则 7 三件套).
//
// Two classes:
//   - Parse-time (malformed file): NO_FRONTMATTER / UNTERMINATED_FRONTMATTER
//     / PARSE_ERROR / TOO_LARGE / TOO_DEEP
//   - Validate-time (well-formed YAML, bad content): MISSING_NAME /
//     INVALID_NAME / MISSING_DESCRIPTION / VALIDATION_FAILED (aggregate)
//   - Namespace-time: NAMESPACE_CONFLICT (same name + same precedence)
//
// Messages/hints are English per 规则 13. Registered at file scope so the
// static manifest scan (cmd/opendbx/errcode_manifest_test.go) and the
// gen-error-codes blank-import both see them.

package skills

import "github.com/sqlrush/opendbx/internal/platform/errcode"

//nolint:gochecknoglobals // spec-0.6 contract: errcode sentinels are package-level.
var (
	// ErrNoFrontmatter — file does not begin with a `---` frontmatter fence.
	ErrNoFrontmatter = errcode.Register(
		"SKILL.NO_FRONTMATTER",
		"SKILL.md does not start with a --- frontmatter fence",
		"begin the file with a YAML frontmatter block delimited by --- lines",
	)
	// ErrUnterminatedFrontmatter — opening `---` has no closing `---` fence.
	ErrUnterminatedFrontmatter = errcode.Register(
		"SKILL.UNTERMINATED_FRONTMATTER",
		"SKILL.md frontmatter is missing its closing --- fence",
		"add a --- line (at column 0) to close the frontmatter before the body",
	)
	// ErrParseError — frontmatter is not valid YAML.
	ErrParseError = errcode.Register(
		"SKILL.PARSE_ERROR",
		"SKILL.md frontmatter is not valid YAML",
		"fix the YAML syntax in the frontmatter block",
	)
	// ErrTooLarge — file exceeds the 1 MiB safety cap (opendbx divergence,
	// not a CC limit).
	ErrTooLarge = errcode.Register(
		"SKILL.TOO_LARGE",
		"SKILL.md exceeds the 1 MiB size limit",
		"split or trim the skill; the 1 MiB cap is an opendbx safety bound",
	)
	// ErrTooDeep — frontmatter YAML nesting depth ≥ 32 (anti-bomb).
	ErrTooDeep = errcode.Register(
		"SKILL.TOO_DEEP",
		"SKILL.md frontmatter YAML nesting is too deep (>= 32)",
		"flatten the frontmatter; deep nesting is rejected as an anti-bomb guard",
	)
	// ErrMissingName — frontmatter has no (non-blank) name.
	ErrMissingName = errcode.Register(
		"SKILL.MISSING_NAME",
		"SKILL.md frontmatter is missing the required name field",
		"add a non-empty name: field; the name is the global skill identifier",
	)
	// ErrInvalidName — name contains path/control characters (loose rule; Q5).
	ErrInvalidName = errcode.Register(
		"SKILL.INVALID_NAME",
		"SKILL.md name contains illegal characters or is too long",
		"use a name without path separators, whitespace, or control chars (<= 128 chars)",
	)
	// ErrMissingDescription — frontmatter has no (non-blank) description.
	ErrMissingDescription = errcode.Register(
		"SKILL.MISSING_DESCRIPTION",
		"SKILL.md frontmatter is missing the required description field",
		"add a description: field; it drives when the model selects this skill",
	)
	// ErrNamespaceConflict — two skills share a name at the same precedence
	// (not resolvable by source ranking).
	ErrNamespaceConflict = errcode.Register(
		"SKILL.NAMESPACE_CONFLICT",
		"two skills share the same name at the same precedence",
		"rename one skill or move it to a different source so precedence can resolve it",
	)
	// ErrValidationFailed — aggregate wrapper over one or more validation
	// failures (Q7 multi-error). Wraps errors.Join(detail...) so the sidecar
	// sees this primary code while details survive under Unwrap.
	ErrValidationFailed = errcode.Register(
		"SKILL.VALIDATION_FAILED",
		"SKILL.md failed one or more validation rules",
		"fix the reported field violations; see the wrapped details for each",
	)
)
