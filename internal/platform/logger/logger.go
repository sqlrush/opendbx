// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package logger implements opendbx's debug logger.
//
// Design: spec-0.5-logger-trace.md
//
// Two-path output model (user 2026-05-10 拍板 B 改良版):
//   - Main user-facing path: 100% Claude Code text format
//     (~/.opendbx/debug/<sessionId>.txt + latest symlink)
//   - JSONL event sidecar (opendbx-only internal):
//     <sessionId>.events.jsonl with trace_id/span_id/session_id/module/event/level/attrs/error
//
// Hard constraint: --debug-file=<path> outputs CC text, never JSON.
// JSONL sidecar path is independent and not affected by --debug-file.
//
// trace_id/span_id reserved schema; OTel SDK swap deferred to spec-3.x via
// wrapper layer (Q10 ★A). spec-0.5 imports stdlib only.
package logger

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
)

// Level enumerates debug log levels (1:1 with CC debug.ts:18).
//
// Order is significant: lower value = higher verbosity. Default min level is
// LevelDebug (filter LevelVerbose).
type Level int8

// Level constants mirror Claude Code debug log levels.
const (
	LevelVerbose Level = iota // 0 — high-volume diagnostics (statusLine cmd, shell, cwd, stdout/stderr)
	LevelDebug                // 1 — default min level
	LevelInfo                 // 2
	LevelWarn                 // 3
	LevelError                // 4
)

// String returns the canonical level name (verbatim CC).
func (l Level) String() string {
	switch l {
	case LevelVerbose:
		return "verbose"
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	default:
		return "unknown"
	}
}

// ParseLevel parses a level name (case-insensitive).
//
// Returns ErrInvalidLevel if name is not one of verbose/debug/info/warn/error.
func ParseLevel(name string) (Level, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "verbose":
		return LevelVerbose, nil
	case "debug":
		return LevelDebug, nil
	case "info":
		return LevelInfo, nil
	case "warn":
		return LevelWarn, nil
	case "error":
		return LevelError, nil
	default:
		return LevelDebug, ErrInvalidLevel
	}
}

// ErrInvalidLevel is returned by ParseLevel for unrecognised names.
var ErrInvalidLevel = errors.New("logger: invalid level")

// Attr is a single key/value attribute attached to a log event.
//
// Attrs are redacted before output (spec § 2.6 D-9): string Values matching
// secret patterns (password=, token=, Authorization, Bearer, sk-*, URL
// userinfo) become <REDACTED>; struct/map Values walk through config.Redact()
// equivalent logic.
type Attr struct {
	Key   string
	Value any
}

// Logger is the public interface for emitting debug events.
//
// All methods are safe for concurrent use. They are no-ops when the logger
// has not been initialised (Init has not been called) — this matches CC's
// "ignore writes before bootstrap" semantics.
type Logger interface {
	Verbose(msg string, attrs ...Attr)
	Debug(msg string, attrs ...Attr)
	Info(msg string, attrs ...Attr)
	Warn(msg string, attrs ...Attr)
	Error(msg string, attrs ...Attr)

	// WithModule returns a derived logger that tags events with the given
	// module category. Affects filter matching (CC filter pattern adapter)
	// and sidecar attrs.
	//
	// Calls chain: WithModule("a").WithModule("b") replaces the category
	// (matches CC behaviour where the latest module name wins).
	WithModule(name string) Logger

	// WithAttrs returns a derived logger that pre-binds the given attrs.
	// Subsequent log calls' attrs are appended.
	WithAttrs(attrs ...Attr) Logger

	// WithContext returns a derived logger that pulls trace_id/span_id from
	// the given context. If ctx has no active span, trace_id/span_id are
	// emitted as empty strings (Q8 ★A).
	WithContext(ctx context.Context) Logger
}

// InitInput holds the parameters required to initialise the global logger.
//
// Pass via Init exactly once per process. Fields not set use defaults
// derived from the environment + argv (CC parity).
type InitInput struct {
	// MinLevel sets the minimum level to emit. Defaults to LevelDebug.
	// Override via OPENDBX_DEBUG_LOG_LEVEL env or callers.
	MinLevel Level

	// SessionID overrides the auto-generated UUID v4 session id. Used by
	// tests and CI for deterministic golden comparison. Empty string means
	// "generate one".
	SessionID string

	// LogPath overrides the default debug log path. If empty, falls back to
	// --debug-file flag, then OPENDBX_DEBUG_LOGS_DIR env, then
	// <configHome>/debug/<sessionId>.txt.
	LogPath string

	// DisableSidecar disables JSONL sidecar emission. The default is enabled
	// (false) so a zero-value InitInput still matches the spec-0.5 contract.
	DisableSidecar bool

	// DebugToStderr forces output to os.Stderr instead of file. Mirrors
	// --debug-to-stderr / -d2e flags.
	DebugToStderr bool
}

// Errors returned by package-level functions.
var (
	// ErrAlreadyInitialised is returned by Init on a second call. Init is
	// idempotent via sync.Once; the first successful call wins.
	ErrAlreadyInitialised = errors.New("logger: already initialised")

	// ErrNotInitialised is returned by Close when called before Init.
	ErrNotInitialised = errors.New("logger: not initialised")
)

// Package-level state (sync.Once guarantees idempotent Init; rule 9 race-clean).
var (
	initOnce  sync.Once
	initErr   error
	initDone  atomic.Bool // true after the once has fired (initErr may still be non-nil)
	closeOnce sync.Once
	current   atomic.Pointer[loggerImpl] // current global logger (nil until Init succeeds)

	// runtimeDebugEnabled lets EnableDebugLogging flip debug mode mid-process
	// without restarting (spec § 2.1 contract; claude HIGH-2 integration).
	// MUST NOT be memoised — every isDebugMode call re-reads.
	runtimeDebugEnabled atomic.Bool

	// hasFormattedOutput controls multi-line message handling (spec § 2.2,
	// codex+claude HIGH-1). When true, multi-line msg → json.Marshal single
	// line (TUI active); when false, verbatim (pre-TUI).
	hasFormattedOutput atomic.Bool
)

// Init initialises the global logger. It is idempotent (sync.Once);
// subsequent calls return ErrAlreadyInitialised without re-initialising.
//
// To reset for tests, use the unexported resetForTesting helper from the
// _test.go files (T-12 will introduce it).
func Init(in InitInput) error {
	initOnce.Do(func() {
		initErr = doInit(in)
		initDone.Store(true)
	})
	if initDone.Load() && initErr == nil {
		// sync.Once already fired with a successful first call; later calls
		// are idempotent successes.
		return nil
	}
	return initErr
}

// doInit performs the actual initialisation; called exactly once via initOnce.
//
// T-3 contract: stand up the logger value with sane defaults so callers can
// emit events without panicking. T-4 onwards extends with real path/output.
func doInit(in InitInput) error {
	if in.MinLevel == LevelVerbose {
		// LevelVerbose is the Go zero value, so a zero-value InitInput should
		// still honor the CC default/env min-level path. Explicit verbose is
		// selected via OPENDBX_DEBUG_LOG_LEVEL=verbose.
		in.MinLevel = getMinDebugLogLevel()
	} else if in.MinLevel < LevelVerbose || in.MinLevel > LevelError {
		in.MinLevel = getMinDebugLogLevel()
	}
	impl := newLoggerImpl(in)
	current.Store(impl)
	return nil
}

// Close flushes and releases all logger resources. It is panic-safe and may
// be called once; subsequent calls are no-ops.
//
// dispose contract (spec § 3, claude HIGH-4): main and sidecar flushes BOTH
// run regardless of individual errors; combined errors returned via
// errors.Join. sidecar Close is best-effort and does not affect process exit
// status (codex MED-3).
func Close() error {
	if !initDone.Load() {
		return ErrNotInitialised
	}
	var err error
	closeOnce.Do(func() {
		impl := current.Load()
		if impl == nil {
			return
		}
		err = impl.close()
	})
	return err
}

// L returns the global logger. Before Init has been called, it returns a
// no-op logger that silently drops all events — this matches CC's pre-bootstrap
// behaviour and lets early code (e.g. flag parsing) reference the logger
// without panicking.
func L() Logger {
	if impl := current.Load(); impl != nil {
		return impl
	}
	return noopLogger{}
}

// EnableDebugLogging flips the runtime debug toggle to true.
//
// Mirrors CC's enableDebugLogging: lets a `/debug` slash command (spec-1.16)
// turn debug on mid-session. Returns the previous value so callers can detect
// "was already enabled".
//
// IMPORTANT: isDebugMode is NOT memoised; this Store is observed immediately
// by the next isDebugMode call (claude HIGH-2 contract).
func EnableDebugLogging() bool {
	return runtimeDebugEnabled.Swap(true)
}

// SetHasFormattedOutput updates the global flag controlling multi-line message
// handling.
//
// spec-1.12 tcell-bootstrap calls SetHasFormattedOutput(true) when the TUI
// renderer starts; calls SetHasFormattedOutput(false) on TUI shutdown. Pre-TUI
// state defaults to false (verbatim multi-line, matching CC's "no
// jsonStringify before TUI active" behaviour).
//
// codex HIGH-1 + claude HIGH-1 integration.
func SetHasFormattedOutput(v bool) {
	hasFormattedOutput.Store(v)
}

// isFormattedOutput returns the current flag value. Used by text_formatter
// (T-4).
func isFormattedOutput() bool {
	return hasFormattedOutput.Load()
}

// isDebugRuntimeEnabled reports whether EnableDebugLogging has been called.
// Called by paths.IsDebugMode (T-6).
func isDebugRuntimeEnabled() bool {
	return runtimeDebugEnabled.Load()
}
