// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package errors registration for the logger package. spec-0.6 D-3+D-4
// migration: the four public sentinels (ErrInvalidLevel /
// ErrAlreadyInitialised / ErrNotInitialised / ErrWriterClosed) are
// re-declared here as errcode.Sentinel values so they carry Code/Message/
// Hint and participate in errors.Is symmetry by Code match.
//
// Backward compatibility: caller code that does
// `errors.Is(err, logger.ErrInvalidLevel)` continues to work — the sentinel
// value's runtime type is *structuredErr (typed as errcode.Sentinel), and
// errors.Is consults its Is(target) method which matches by Code.

package logger

import "github.com/sqlrush/opendbx/internal/platform/errcode"

// Note: the existing var declarations of these sentinels in logger.go /
// buffered_writer.go are removed in this commit and rewired to point at the
// errcode.Register values defined here. Sentinel names + errors.Is usage
// stay backward-compatible.

//nolint:gochecknoglobals // spec-0.6 § 2.2.1 canonical Register pattern
var (
	// ErrInvalidLevel — ParseLevel rejected an unknown level name.
	ErrInvalidLevel errcode.Sentinel = errcode.Register(
		"LOGGER.INVALID_LEVEL",
		"invalid log level name",
		"use one of: verbose, debug, info, warn, error",
	)

	// ErrAlreadyInitialised — second Init call after a successful first call.
	ErrAlreadyInitialised errcode.Sentinel = errcode.Register(
		"LOGGER.ALREADY_INITIALISED",
		"logger already initialised",
		"Init is sync.Once-guarded; benign no-op for cobra inheritance chains",
	)

	// ErrNotInitialised — Close called before Init.
	ErrNotInitialised errcode.Sentinel = errcode.Register(
		"LOGGER.NOT_INITIALISED",
		"logger not initialised",
		"call logger.Init before Close / L() on the global instance",
	)

	// ErrWriterClosed — BufferedWriter.Write after Dispose.
	ErrWriterClosed errcode.Sentinel = errcode.Register(
		"LOGGER.WRITER_CLOSED",
		"BufferedWriter has been closed",
		"do not write to a disposed writer; use a fresh logger.Init or skip emission",
	)
)
