// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package logger

import (
	"context"
	"sync/atomic"
	"time"
)

// Span represents an in-flight trace span.
//
// Design (spec § 2.4): the interface is opendbx-original and intentionally
// distinct from go.opentelemetry.io/otel/trace.Span (which uses
// SetAttributes / RecordError / End with variadic option args). spec-3.x will
// introduce a wrapper layer that translates this interface into OTel's; the
// opendbx-side call sites remain unchanged when that swap happens.
//
// Implementations must be safe for concurrent use across goroutines that
// share the ctx returned by StartSpan.
type Span interface {
	// SetAttr attaches a key/value to the span. The value is included in the
	// span.end sidecar event under attrs.
	SetAttr(key string, value any)

	// RecordError marks the span as carrying an error condition. The error's
	// message is preserved on End via the sidecar `error.message` field.
	// Subsequent RecordError calls overwrite the previous error.
	RecordError(err error)

	// End closes the span and emits a span.end event to the sidecar with
	// duration_ms, attrs, and (if any) the recorded error. Safe to call
	// multiple times; only the first End has effect (idempotent).
	End()
}

// spanCtxKey is the context.Context key under which the active span is
// stored. Unexported sentinel type per Go idiom.
type spanCtxKey struct{}

// span is the concrete Span implementation.
type span struct {
	traceID      string
	spanID       string
	parentSpanID string
	verb         string
	startTime    time.Time

	mu       *atomic.Pointer[spanState] // pointer so With* clones share state
	emitFunc func(traceID, spanID, parentSpanID, verb string, start, end time.Time, attrs map[string]any, err error)
}

// spanState holds the mutable per-span data (attrs map + error + ended flag).
// Stored behind an atomic.Pointer so reads from concurrent goroutines see a
// consistent snapshot without a per-call lock.
type spanState struct {
	attrs map[string]any
	err   error
	ended bool
}

// StartSpan opens a new span. If ctx already carries an active span, the new
// span's parent_span_id is set to that span's id and the trace_id is
// inherited (so all spans in a request share one trace_id).
//
// The returned ctx must be passed downstream — any child StartSpan call or
// logger.WithContext / Info call uses it to look up the current trace_id /
// span_id. Returning a nil span (panic-safe) is never done; callers can
// always invoke End() without nil-checking.
//
// span.End() is responsible for emitting the span.end sidecar event. Callers
// should typically `defer span.End()` immediately after StartSpan.
func StartSpan(ctx context.Context, verb string) (context.Context, Span) {
	if ctx == nil {
		ctx = context.Background()
	}
	parent := spanFromContext(ctx)
	traceID := uuid7()
	parentSpanID := ""
	if parent != nil {
		traceID = parent.traceID
		parentSpanID = parent.spanID
	}
	state := &atomic.Pointer[spanState]{}
	state.Store(&spanState{attrs: map[string]any{}})

	s := &span{
		traceID:      traceID,
		spanID:       uuid7(),
		parentSpanID: parentSpanID,
		verb:         verb,
		startTime:    time.Now(),
		mu:           state,
		emitFunc:     emitSpanEnd,
	}
	return context.WithValue(ctx, spanCtxKey{}, s), s
}

// SetAttr stores key/value on the span. Concurrent SetAttr calls are
// serialised via the atomic.Pointer state swap: we read-copy-write the map,
// then CAS the pointer. Retry on CAS conflict.
func (s *span) SetAttr(key string, value any) {
	for {
		old := s.mu.Load()
		if old == nil || old.ended {
			return
		}
		// Copy-on-write: build a new map containing old + new entry.
		newAttrs := make(map[string]any, len(old.attrs)+1)
		for k, v := range old.attrs {
			newAttrs[k] = v
		}
		newAttrs[key] = value
		next := &spanState{attrs: newAttrs, err: old.err, ended: false}
		if s.mu.CompareAndSwap(old, next) {
			return
		}
	}
}

// RecordError stores err on the span. Same CAS pattern as SetAttr.
// Subsequent RecordError calls overwrite (matches OTel behaviour: last error
// is the one reported).
func (s *span) RecordError(err error) {
	for {
		old := s.mu.Load()
		if old == nil || old.ended {
			return
		}
		next := &spanState{attrs: old.attrs, err: err, ended: false}
		if s.mu.CompareAndSwap(old, next) {
			return
		}
	}
}

// End closes the span and emits the span.end sidecar event. Idempotent:
// only the first call has effect.
func (s *span) End() {
	for {
		old := s.mu.Load()
		if old == nil || old.ended {
			return // already ended
		}
		next := &spanState{attrs: old.attrs, err: old.err, ended: true}
		if s.mu.CompareAndSwap(old, next) {
			if s.emitFunc != nil {
				s.emitFunc(s.traceID, s.spanID, s.parentSpanID, s.verb, s.startTime, time.Now(), old.attrs, old.err)
			}
			return
		}
	}
}

// spanFromContext returns the span carried in ctx, or nil if none. Used by
// StartSpan to determine parent + by traceIDsFromContext to populate sidecar
// trace/span fields.
func spanFromContext(ctx context.Context) *span {
	if ctx == nil {
		return nil
	}
	v := ctx.Value(spanCtxKey{})
	if v == nil {
		return nil
	}
	s, _ := v.(*span)
	return s
}

// emitSpanEnd is the package-level callback invoked by Span.End. It
// constructs a `span.end` sidecar event with duration_ms and forwards it
// through the current global logger's sidecar writer.
//
// If no logger is initialised, the call is a no-op (matches the "L() is
// safe before Init" contract). Errors are best-effort: a sidecar marshal /
// write failure here would, like all sidecar paths, only stderr-warn.
func emitSpanEnd(traceID, spanID, parentSpanID, verb string, start, end time.Time, attrs map[string]any, recErr error) {
	impl := current.Load()
	if impl == nil || !impl.sidecarEnabled || impl.sidecarWriter == nil {
		return
	}

	merged := []Attr{
		{Key: "trace_id", Value: traceID},
		{Key: "span_id", Value: spanID},
		{Key: "event", Value: "span.end"},
		{Key: "duration_ms", Value: end.Sub(start).Milliseconds()},
		{Key: "verb", Value: verb},
	}
	if parentSpanID != "" {
		merged = append(merged, Attr{Key: "parent_span_id", Value: parentSpanID})
	}
	for k, v := range attrs {
		// Skip reserved keys; the explicit Attr entries above win.
		if _, reserved := reservedAttrKeys[k]; reserved {
			continue
		}
		merged = append(merged, Attr{Key: k, Value: v})
	}

	// codex CRIT-2 integration: redact span attrs BEFORE JSON serialisation.
	// The post-format `redactString` cannot detect secrets inside JSON-encoded
	// values (e.g. `"password":"hunter2"` does not match the key=value regex).
	// The pre-format pass on attrs is the only reliable secret defence here.
	merged = redactAttrs(merged)

	line, err := marshalSidecarEvent(end, LevelInfo, impl.module, redactString(verb+" span ended"), impl.sessionID, merged, traceID, spanID)
	if err != nil {
		warnSidecar("marshal-span", getSidecarPath(impl.sessionID), err)
		return
	}
	// Attach error info if RecordError was called. The error message is
	// redacted before embedding so the sidecar never carries a raw secret
	// even when callers do `RecordError(fmt.Errorf("auth failed: token=%s", t))`.
	if recErr != nil {
		line, _ = injectSpanError(line, redactedError{msg: redactString(recErr.Error())})
	}
	// Post-format redaction (spec § 2.6 fail-safe layer): also catches
	// secrets that leaked through attrs.
	_ = impl.sidecarWriter.Write(redactString(string(line)))
}

// injectSpanError rewrites a sidecar JSON line to populate the `error` field
// from a Go error. spec-0.6 will introduce a structured error type with
// Code/Hint; for now we map .Error() into `error.message` only.
func injectSpanError(line []byte, recErr error) ([]byte, error) {
	const needle = `"error":null`
	idx := indexOfLastBytes(line, needle)
	if idx < 0 {
		return line, nil
	}
	replacement := []byte(`"error":{"code":"","message":` + jsonString(recErr.Error()) + `,"hint":""}`)
	out := make([]byte, 0, len(line)-len(needle)+len(replacement))
	out = append(out, line[:idx]...)
	out = append(out, replacement...)
	out = append(out, line[idx+len(needle):]...)
	return out, nil
}

// indexOfLastBytes returns the index of the last occurrence of needle in src,
// or -1 if absent. Plain byte scan; the JSON line is short.
func indexOfLastBytes(src []byte, needle string) int {
	if len(needle) == 0 || len(src) < len(needle) {
		return -1
	}
	n := []byte(needle)
	for i := len(src) - len(n); i >= 0; i-- {
		if bytesEqual(src[i:i+len(n)], n) {
			return i
		}
	}
	return -1
}

// bytesEqual is a small helper kept package-local to avoid importing "bytes"
// solely for this purpose.
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// jsonString escapes s as a JSON string literal (including surrounding
// quotes). Uses encoding/json under the hood for correctness on edge cases
// (control chars, unicode, embedded quotes).
func jsonString(s string) string {
	// We allocate but the path is per-error, not per-event; cost is fine.
	b, err := jsonMarshalString(s)
	if err != nil {
		return `""`
	}
	return string(b)
}
