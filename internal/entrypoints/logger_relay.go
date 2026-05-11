// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Logger relay (spec-0.5 D-8). Routes cmd/opendbx → internal/platform/logger
// through entrypoints so that the cmd → platform exception remains
// internal/platform/version only (per spec-0.3 hotfix rationale).
//
// Pattern mirrors config_relay.go.

package entrypoints

import (
	"errors"

	"github.com/sqlrush/opendbx/internal/platform/logger"
)

// LoggerInitInputs carries the cobra-parsed knobs that drive logger.Init.
//
// SessionID is optional — empty means "let logger generate a UUID v4".
// LogPath similarly defers to logger's path-resolution chain when empty
// (--debug-file → OPENDBX_DEBUG_LOGS_DIR env → <configHome>/debug/...).
type LoggerInitInputs struct {
	SessionID      string
	LogPath        string
	DebugToStderr  bool
	DisableSidecar bool
}

// InitLoggerFromCLI initialises the global logger using the cobra-parsed
// inputs. Called from cmd/opendbx PersistentPreRunE after config has been
// loaded.
//
// Idempotent (logger.Init is sync.Once-guarded internally); subsequent
// calls are no-ops returning nil.
//
// NOTE (spec-0.5 → spec-0.4 forward link): spec-0.5 § 1.4 originally
// planned cfg.Output.LogLevel / cfg.Output.LogPath plumbing here. spec-0.4
// shipped without those fields (they were not in the agreed 7-sub-struct
// shape). Until a spec-0.4 errata or spec-0.6 adds them, logger.Init
// resolves level via OPENDBX_DEBUG_LOG_LEVEL env and path via --debug-file
// / OPENDBX_DEBUG_LOGS_DIR — all already wired in paths.go (T-6).
func InitLoggerFromCLI(in LoggerInitInputs) error {
	err := logger.Init(logger.InitInput{
		SessionID:      in.SessionID,
		LogPath:        in.LogPath,
		DebugToStderr:  in.DebugToStderr,
		DisableSidecar: in.DisableSidecar,
	})
	// ErrAlreadyInitialised is a benign no-op for cobra inheritance chains
	// (PersistentPreRunE inherits into subcommands; each invocation hits this
	// relay once). Callers that need to distinguish "fresh init" can check
	// for it directly.
	if errors.Is(err, logger.ErrAlreadyInitialised) {
		return nil
	}
	return err
}

// RegisterLoggerSignalCleanup arms SIGINT / SIGTERM handlers so the logger
// flushes before the process exits. Idempotent. Called once at startup
// after InitLoggerFromCLI.
func RegisterLoggerSignalCleanup() {
	logger.RegisterSignalCleanup()
}

// CloseLogger flushes and disposes the global logger. Safe to call multiple
// times; returns ErrNotInitialised if Init was never invoked.
func CloseLogger() error {
	return logger.Close()
}

// GuardLoggerPanic wraps fn with panic recovery that records a process.panic
// event + flushes both writers. Re-panics after.
func GuardLoggerPanic(fn func()) {
	logger.GuardPanic(fn)
}
