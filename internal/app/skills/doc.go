// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package skills defines the SKILL.md file format: frontmatter schema,
// parsing, validation, and namespace resolution. spec-2.1-skill-md-format.
//
// Scope (spec-2.1): PURE format + validation, NO filesystem I/O.
//   - schema.go    — Schema (CC frontmatter mirror) + AllowedToolsList
//   - skill.go     — Skill value + SkillSource provenance + Key identity
//   - parse.go     — Parse: frontmatter split (yaml.Node) + Extra + caps
//   - validate.go  — Validate → ValidationReport (warnings as data) + errors
//   - namespace.go — Resolve → Resolution (precedence + conflict kinds)
//   - errors.go    — SKILL.* errcode sentinels
//
// Out of scope (follow-on specs): discovery / hot-reload (spec-2.2),
// invocation / SkillTool / allowed-tools enforcement (spec-2.3), the
// improvement loop (spec-2.4). This package is a leaf: it imports only
// errcode + go.yaml.in/yaml/v3 + stdlib.
//
// Design intent: one SKILL.md loads byte-faithfully in BOTH Claude Code and
// opendbx. The frontmatter schema mirrors CC (survey
// docs/surveys/cc-ecosystem/skills.md, CC SHA 3da94d5); opendbx extension
// fields (opendbx_*) are preserved forward-compatibly in Schema.Extra rather
// than frozen as typed fields (Q6=B; §3.7 append-only).
package skills
