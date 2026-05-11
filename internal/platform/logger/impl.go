// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package logger

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
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
	filter         *debugFilter
	mainWriter     *bufferedWriter
	sidecarWriter  *bufferedWriter // independent JSONL sidecar (T-7 D-5); nil if disabled

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
	logPath := in.LogPath
	if logPath == "" {
		logPath = getDebugLogPath(sid)
	}
	debugToStderr := in.DebugToStderr || isDebugToStdErr()
	cfg := defaultBufferedWriterConfig(mainWriteFunc(logPath, debugToStderr))
	cfg.immediateMode = IsDebugMode() || debugToStderr
	sidecarEnabled := !in.DisableSidecar
	var sidecar *bufferedWriter
	if sidecarEnabled {
		// Sidecar path is intentionally NOT derived from in.LogPath / --debug-file
		// (Q3 ★A hard constraint): the user-facing flag controls only the main
		// text surface; sidecar always lands under the platform debug dir keyed
		// by session id so machine consumers (sentinel / CI) can find it.
		sidecar = newSidecarWriter(getSidecarPath(sid))
	}
	return &loggerImpl{
		mu:             &sync.Mutex{},
		minLevel:       in.MinLevel,
		sessionID:      sid,
		logPath:        logPath,
		sidecarEnabled: sidecarEnabled,
		debugToStderr:  debugToStderr,
		filter:         getDebugFilter(),
		mainWriter:     newBufferedWriter(cfg),
		sidecarWriter:  sidecar,
	}
}

func mainWriteFunc(logPath string, debugToStderr bool) writeFunc {
	if debugToStderr {
		return func(content string) error {
			_, err := os.Stderr.WriteString(content)
			return err
		}
	}
	// Per-writer sync.Once guards the latest-link creation so it runs exactly
	// once per logger lifecycle, on the first successful write (claude HIGH-3
	// lazy-memoize semantics — never eager at Init, never repeated).
	//
	// Q4 ★A R3 constraint #4: sidecar uses its own writeFunc (sidecarWriteFunc)
	// and is intentionally NOT given a latest link. Only the main text path
	// participates so a `--debug-file=<custom>` invocation keeps the latest
	// link tracking the user-facing surface.
	var linkOnce sync.Once
	return func(content string) error {
		if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
			return err
		}
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) //nolint:gosec // debug path is operator-controlled
		if err != nil {
			return err
		}
		_, writeErr := f.WriteString(content)
		closeErr := f.Close()
		if writeErr != nil {
			return writeErr
		}
		if closeErr == nil {
			// Lazy-once: maintain latest only after the first successful write.
			linkOnce.Do(func() { updateLatestLink(logPath) })
		}
		return closeErr
	}
}

// generateSessionID returns a fresh UUID v4 (spec § 8 Q16: sessionId is v4;
// trace_id is v7). Implemented from crypto/rand to keep the package
// stdlib-only per spec § 5 contract.
func generateSessionID() string {
	return uuid4()
}

// close flushes and releases all writer resources.
//
// Dispose contract (spec § 3, claude HIGH-4 + codex MED-3):
//   - BOTH the main writer and the sidecar writer are flushed/closed even if
//     one fails. We never early-return on an intermediate error.
//   - Errors are combined via errors.Join so callers can inspect each leg
//     independently (e.g. main path disk-full + sidecar permission-denied).
//   - sidecar errors are best-effort and should not by themselves change
//     process exit status; we surface them here for completeness, but the
//     sidecar write path itself already swallows write failures to stderr
//     (see sidecarWriteFunc). So sidecar Dispose typically returns nil.
func (l *loggerImpl) close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	var mainErr, sideErr error
	if l.mainWriter != nil {
		mainErr = l.mainWriter.Dispose()
	}
	if l.sidecarWriter != nil {
		sideErr = l.sidecarWriter.Dispose()
	}
	return errors.Join(mainErr, sideErr)
}

// log is the central event emission funnel. Pipeline:
//  1. level check (vs configured minLevel)
//  2. debug-mode gate (IsDebugMode or debugToStderr; mirrors CC's "no
//     writes outside debug mode" contract)
//  3. filter check (D-3)
//  4. text formatter → main BufferedWriter (D-2)
//  5. sidecar JSONL marshal → independent BufferedWriter (D-5)
//
// Redaction (D-9) and trace_context (D-6) hook into steps 4/5 via T-8 / T-10
// follow-ups; T-7 plumbs in the schema fields with empty trace_id/span_id
// (Q8 ★A behaviour for events outside an active span).
func (l *loggerImpl) log(level Level, msg string, attrs []Attr) {
	if level < l.minLevel {
		return
	}
	if !IsDebugMode() && !l.debugToStderr {
		return
	}
	if !l.shouldShow(msg) {
		return
	}
	now := time.Now()
	merged := mergeAttrs(l.attrs, attrs)

	// Main text path. Best-effort: write errors are swallowed (the BufferedWriter
	// itself propagates them to its caller, but logger.Error()-style recursion
	// would deadlock on the same goroutine — keep it simple).
	if l.mainWriter != nil {
		_ = l.mainWriter.Write(formatEvent(now, level, msg))
	}

	// Sidecar JSONL path (independent file handle / buffer; failures do NOT
	// affect the main path per spec § 3 guarantee).
	if l.sidecarEnabled && l.sidecarWriter != nil {
		ctxTraceID, ctxSpanID := traceIDsFromContext(l.ctx)
		line, err := marshalSidecarEvent(now, level, l.module, msg, l.sessionID, merged, ctxTraceID, ctxSpanID)
		if err != nil {
			warnSidecar("marshal", l.sessionID, err)
		} else {
			_ = l.sidecarWriter.Write(string(line))
		}
	}
}

// traceIDsFromContext extracts the trace_id / span_id pair carried by an
// active span in ctx (via StartSpan). Returns ("", "") when no span is
// active — Q8 ★A: events outside an active span emit explicit empty
// strings rather than synthesising fake UUIDs.
func traceIDsFromContext(ctx context.Context) (traceID, spanID string) {
	if s := spanFromContext(ctx); s != nil {
		return s.traceID, s.spanID
	}
	return "", ""
}

func (l *loggerImpl) shouldShow(msg string) bool {
	if l.filter == nil {
		return true
	}
	categories := extractDebugCategories(msg)
	if l.module != "" {
		categories = append(categories, l.module)
	}
	return shouldShowDebugCategories(categories, l.filter)
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
