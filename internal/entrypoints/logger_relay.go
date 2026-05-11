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
	"strings"

	"github.com/sqlrush/opendbx/internal/platform/config"
	"github.com/sqlrush/opendbx/internal/platform/logger"
)

// LoggerInitInputs carries the cobra-parsed knobs that drive logger.Init.
//
// SessionID is optional — empty means "let logger generate a UUID v4".
// LogPath similarly defers to logger's path-resolution chain when empty
// (--debug-file → OPENDBX_DEBUG_LOGS_DIR env → <configHome>/debug/...).
type LoggerInitInputs struct {
	SessionID      string
	Debug          string
	DebugFile      string
	LogPath        string
	MinLevel       string
	DebugToStderr  bool
	DisableSidecar bool
}

// InitLoggerFromConfigAndCLI initialises the global logger using config plus
// cobra-parsed inputs. Explicit CLI debug flags win over config defaults.
func InitLoggerFromConfigAndCLI(cfg *config.Config, in LoggerInitInputs) error {
	initInput := logger.InitInput{
		SessionID:      in.SessionID,
		DebugEnabled:   in.Debug != "" || in.DebugFile != "" || in.DebugToStderr,
		DebugFilter:    in.Debug,
		DebugToStderr:  in.DebugToStderr,
		DisableSidecar: in.DisableSidecar,
	}

	switch {
	case in.DebugFile != "":
		initInput.LogPath = in.DebugFile
	case in.LogPath != "":
		initInput.LogPath = in.LogPath
	case cfg != nil && cfg.Output.LogPath != "":
		initInput.LogPath = cfg.Output.LogPath
	}

	minLevel := strings.TrimSpace(in.MinLevel)
	if minLevel == "" && cfg != nil &&
		cfg.Output.LogLevel != "" &&
		cfg.Source("Output.LogLevel") != config.SourceDefault {
		minLevel = cfg.Output.LogLevel
	}
	if minLevel != "" {
		lvl, err := logger.ParseLevel(minLevel)
		if err != nil {
			return err
		}
		initInput.MinLevel = lvl
		initInput.MinLevelSet = true
	}

	err := logger.Init(initInput)
	if errors.Is(err, logger.ErrAlreadyInitialised) {
		return nil
	}
	return err
}

// InitLoggerFromCLI initialises the global logger using the cobra-parsed
// inputs. Called from cmd/opendbx PersistentPreRunE after config has been
// loaded.
//
// Idempotent (logger.Init is sync.Once-guarded internally); subsequent
// calls are no-ops returning nil.
func InitLoggerFromCLI(in LoggerInitInputs) error {
	return InitLoggerFromConfigAndCLI(nil, in)
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
