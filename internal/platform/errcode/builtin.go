// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package errcode

// Builtin codes shared across opendbx packages. Per-package codes live in
// their own errors.go (e.g. internal/platform/logger/errors.go). file-scope
// var = Register is the canonical registration pattern (spec § 2.2.1, codex
// MED-1 R2 alignment).

//nolint:gochecknoglobals // spec-0.6 § 2.2.1 canonical Register pattern
var (
	// ErrInvalidArgument signals that a caller passed an argument that
	// failed validation outside the scope of more specific codes.
	ErrInvalidArgument = Register(
		"ERRCODE.INVALID_ARGUMENT",
		"invalid argument",
		"check the function/API documentation for the expected shape",
	)

	// ErrNotFound signals that a lookup target does not exist.
	ErrNotFound = Register(
		"ERRCODE.NOT_FOUND",
		"requested entity not found",
		"verify the identifier; if managing config / connections / sessions, run the relevant list command",
	)

	// ErrNotImplemented signals a feature that is registered in the spec
	// roadmap but not yet wired up at the current stage.
	ErrNotImplemented = Register(
		"ERRCODE.NOT_IMPLEMENTED",
		"feature not implemented at this stage",
		"check the spec id linked from the call site for the target stage",
	)

	// ErrInternal is a catch-all for unexpected internal failures. Prefer
	// more specific codes wherever possible — this is intended only as a
	// safety net for paths that genuinely cannot be classified.
	ErrInternal = Register(
		"ERRCODE.INTERNAL",
		"internal error",
		"this indicates an opendbx bug; please report with --debug log + sidecar JSONL",
	)

	// ErrFlagInvalid is used by cmd/opendbx for cobra flag value validation.
	// It lives here instead of cmd/opendbx so docs_gen can load it without
	// importing a command package.
	ErrFlagInvalid = Register(
		"CMD.FLAG_INVALID",
		"invalid command-line flag value",
		"check the flag help text and retry with one of the accepted values",
	)
)
