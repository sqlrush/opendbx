// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Registered error codes for the version package. spec-0.7 D-1 + spec-0.6
// contract: every public error returned by Parse() carries a Code/Message/Hint
// triple via errcode.Sentinel.

package version

import "github.com/sqlrush/opendbx/internal/platform/errcode"

//nolint:gochecknoglobals // spec-0.6 § 2.2.1 canonical Register pattern
var (
	// ErrTagInvalid — version tag failed grammar OR semantic validation.
	// Grammar: VersionPattern (regex) match.
	// Semantic: MINOR == spec-registry ordinal; Stage <= 9.
	ErrTagInvalid errcode.Sentinel = errcode.Register(
		"VERSION.TAG_INVALID",
		"version tag invalid",
		"tag must match v<MAJOR>.<MINOR>.<PATCH>-stage<S>.<N>[-accepted] and MINOR must equal spec-registry ordinal for (stage, specN)",
	)
)
