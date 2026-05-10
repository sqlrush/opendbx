// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package logger

import (
	"context"
	"sync"
)

// loggerImpl is the concrete Logger implementation. T-3 establishes the
// struct shape and basic field threading (module / attrs / ctx);
// T-4 onwards plumbs in the formatter, BufferedWriter, sidecar, and trace
// context propagation.
type loggerImpl struct {
	// mu guards mutable runtime state during reconfigure (rare). Stored as
	// *sync.Mutex so With*-derived clones share the lock with the parent
	// rather than each carrying an independent (silently broken) copy.
	mu *sync.Mutex

	minLevel       Level
	sessionID      string
	logPath        string
	sidecarEnabled bool
	debugToStderr  bool

	// Per-call derived state (clones via With* methods).
	module string
	attrs  []Attr
	ctx    context.Context //nolint:containedctx // intentional: WithContext binds ctx into the logger value (T-8)
}

// newLoggerImpl constructs a fresh impl from the validated InitInput.
// SessionID generation, path resolution, and writer construction are split
// into helpers that T-4..T-9 will wire up; T-3 supplies a minimal default.
func newLoggerImpl(in InitInput) *loggerImpl {
	sid := in.SessionID
	if sid == "" {
		sid = generateSessionID()
	}
	return &loggerImpl{
		mu:             &sync.Mutex{},
		minLevel:       in.MinLevel,
		sessionID:      sid,
		logPath:        in.LogPath,
		sidecarEnabled: in.SidecarEnabled,
		debugToStderr:  in.DebugToStderr,
	}
}

// generateSessionID returns a UUID v4 (spec § 8 Q16: sessionId v4, trace_id
// v7). T-8 fills in real RFC 4122 v4 generation; T-3 uses a placeholder so
// the type compiles end-to-end.
func generateSessionID() string {
	return "00000000-0000-4000-8000-000000000000" // T-8 placeholder
}

// close flushes and releases all writer resources. T-7 plumbs in real
// BufferedWriter dispose with errors.Join (claude HIGH-4 contract).
func (l *loggerImpl) close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	// T-7 will:
	//   mainErr := l.mainBW.Flush(); l.mainBW.Close()
	//   sideErr := l.sidecarBW.Flush(); l.sidecarBW.Close()
	//   return errors.Join(mainErr, sideErr)
	return nil
}

// log is the central event emission funnel. T-4 onwards adds:
//   - filter check (D-3)
//   - level check vs minLevel
//   - text formatter → BufferedWriter
//   - sidecar JSONL marshal → independent BufferedWriter
//   - redaction pre/post-format (D-9)
func (l *loggerImpl) log(level Level, msg string, attrs []Attr) {
	if level < l.minLevel {
		return
	}
	// T-4 onwards: real output. T-3 is a no-op so the interface compiles.
	_ = msg
	_ = attrs
}

// Verbose, Debug, Info, Warn, Error all funnel into log.
func (l *loggerImpl) Verbose(msg string, attrs ...Attr) { l.log(LevelVerbose, msg, attrs) }
func (l *loggerImpl) Debug(msg string, attrs ...Attr)   { l.log(LevelDebug, msg, attrs) }
func (l *loggerImpl) Info(msg string, attrs ...Attr)    { l.log(LevelInfo, msg, attrs) }
func (l *loggerImpl) Warn(msg string, attrs ...Attr)    { l.log(LevelWarn, msg, attrs) }
func (l *loggerImpl) Error(msg string, attrs ...Attr)   { l.log(LevelError, msg, attrs) }

// WithModule returns a derived logger tagged with the given module name.
// Calls chain: WithModule("a").WithModule("b") replaces the category — the
// latest module name wins (matches CC's category override behaviour).
func (l *loggerImpl) WithModule(name string) Logger {
	clone := l.clone()
	clone.module = name
	return clone
}

// WithAttrs returns a derived logger that pre-binds the given attrs.
func (l *loggerImpl) WithAttrs(attrs ...Attr) Logger {
	clone := l.clone()
	if len(attrs) == 0 {
		return clone
	}
	merged := make([]Attr, 0, len(clone.attrs)+len(attrs))
	merged = append(merged, clone.attrs...)
	merged = append(merged, attrs...)
	clone.attrs = merged
	return clone
}

// WithContext returns a derived logger carrying the given ctx for trace_id /
// span_id propagation (T-8).
func (l *loggerImpl) WithContext(ctx context.Context) Logger {
	clone := l.clone()
	clone.ctx = ctx
	return clone
}

// clone returns a shallow copy of the impl preserving the underlying writer
// state. The mutex is shared via pointer (writers are package-level singletons
// in the actual T-4..T-9 wiring).
func (l *loggerImpl) clone() *loggerImpl {
	c := *l
	return &c
}
