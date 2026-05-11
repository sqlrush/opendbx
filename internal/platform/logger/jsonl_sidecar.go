// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package logger

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// sidecarEvent is the on-wire shape of a single JSONL sidecar record.
//
// Field order is fixed per spec § 2.3 (Q8 ★A schema stability contract):
// downstream consumers (sentinel / diagnose / CI jq queries) depend on the
// order remaining stable. Future specs may APPEND fields but MUST NOT
// rename or reorder existing ones.
//
// Empty trace_id / span_id values are emitted as "" (not null / not omitted)
// per Q8 ★A: events outside an active span carry explicit empty strings so
// jq can distinguish "in-span" vs "out-of-span" without nil-coalescing.
type sidecarEvent struct {
	Ts        string         `json:"ts"`
	Level     string         `json:"level"`
	Module    string         `json:"module"`
	Event     string         `json:"event"`
	Msg       string         `json:"msg"`
	TraceID   string         `json:"trace_id"`
	SpanID    string         `json:"span_id"`
	SessionID string         `json:"session_id"`
	Attrs     map[string]any `json:"attrs"`
	Error     *sidecarError  `json:"error"`
}

// sidecarError encodes an opendbx error triple (spec § 1.5 + CLAUDE.md 规则
// 7 + spec-0.6 错误码注册表 forward-link). Emitted as JSON `null` when no
// error is associated with the event.
type sidecarError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint"`
}

// reservedAttrKeys are extracted from caller-supplied Attrs and promoted to
// top-level sidecar fields rather than echoed into the `attrs` map.
//
// This lets call sites use uniform attr passing — e.g.
// `logger.L().WithAttrs(Attr{"event", "stream.delta"}).Info("...")` — while
// still surfacing as the top-level `event` field per schema.
var reservedAttrKeys = map[string]struct{}{
	"event":    {},
	"trace_id": {},
	"span_id":  {},
}

// getSidecarPath returns the sidecar JSONL path for the given session id.
//
// CRITICAL (Q3 ★A user hard constraint 2026-05-10): this path is INDEPENDENT
// of `--debug-file`. Even if the user redirected the main text log via
// `--debug-file=/tmp/foo.log`, the sidecar still lands under the default
// debug dir so machine consumers can find it by session id. The flag only
// controls the user-facing text surface; sidecar is internal.
func getSidecarPath(sessionID string) string {
	if sessionID == "" {
		sessionID = "session"
	}
	return filepath.Join(debugDirDefault(), sessionID+".events.jsonl")
}

// newSidecarWriter constructs the BufferedWriter that drains sidecar events
// to disk. Configuration deviations from the main path:
//
//   - immediateMode is ALWAYS false. Sidecar consumers (sentinel / CI / jq)
//     read post-hoc or tail on demand; live-flushing offers no value and adds
//     I/O overhead per event.
//   - The write function never writes to os.Stderr (Q9 ★A: sidecar must not
//     pollute the user-facing stderr stream even when --debug-to-stderr is
//     set on the main path).
//   - Write errors are best-effort (codex MED-3): logged to os.Stderr without
//     recursive logger.Error() (to avoid deadlock) and swallowed so the main
//     path is unaffected. The writeFunc returns nil so BufferedWriter does
//     not retain the error for later Flush.
func newSidecarWriter(path string) *bufferedWriter {
	return newBufferedWriter(bufferedWriterConfig{
		writeFn:        sidecarWriteFunc(path),
		flushInterval:  1000 * time.Millisecond,
		maxBufferSize:  100,
		maxBufferBytes: 0, // honour CC default (math.MaxInt) via newBufferedWriter normalisation
		immediateMode:  false,
	})
}

// sidecarWriteFunc returns a writeFunc that appends content to path.
//
// best-effort contract (spec § 3 + codex MED-3): mkdir / open / write /
// close failures are reported to os.Stderr immediately (so the operator
// sees them) AND surfaced via the writeFunc return value so that
// `BufferedWriter.Dispose()` can join them into the errors.Join result —
// closing codex HIGH-2 ("sidecar errors swallowed before dispose can join
// them"). We still do NOT call back into the logger (would recurse via
// Dispose → another sidecar write).
func sidecarWriteFunc(path string) writeFunc {
	return func(content string) error {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			warnSidecar("mkdir", path, err)
			return err
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) //nolint:gosec // path resolved from session id under config home
		if err != nil {
			warnSidecar("open", path, err)
			return err
		}
		var firstErr error
		if _, werr := f.WriteString(content); werr != nil {
			warnSidecar("write", path, werr)
			firstErr = werr
		}
		if cerr := f.Close(); cerr != nil {
			warnSidecar("close", path, cerr)
			if firstErr == nil {
				firstErr = cerr
			}
		}
		return firstErr
	}
}

// warnSidecar writes a one-line sidecar diagnostic to os.Stderr. It does
// NOT route through the logger (recursion would deadlock the in-flight log
// call that triggered the sidecar write).
func warnSidecar(op, path string, err error) {
	_, _ = os.Stderr.WriteString("opendbx: sidecar " + op + " err (" + path + "): " + err.Error() + "\n")
}

// marshalSidecarEvent constructs and JSON-encodes one sidecar record.
//
// merged is the combined Attr list from WithAttrs-derived bindings and the
// per-call attrs slice. Reserved keys (event / trace_id / span_id) are
// promoted to top-level sidecar fields; the remainder lands in `attrs`.
//
// ctxTraceID and ctxSpanID come from logger.WithContext — they override
// caller-supplied trace_id/span_id attrs ONLY if the ctx values are
// non-empty (so explicit attrs win when no active span). T-8 plumbs in
// real context propagation; T-7 callers pass "" / "".
//
// Returns a newline-terminated JSON line ready to be written.
func marshalSidecarEvent(
	now time.Time,
	level Level,
	module, msg, sessionID string,
	merged []Attr,
	ctxTraceID, ctxSpanID string,
) ([]byte, error) {
	ev := sidecarEvent{
		Ts:        now.UTC().Format(time.RFC3339Nano),
		Level:     level.String(),
		Module:    module,
		Event:     "",
		Msg:       msg,
		TraceID:   ctxTraceID,
		SpanID:    ctxSpanID,
		SessionID: sessionID,
		Attrs:     map[string]any{},
		Error:     nil,
	}

	for _, a := range merged {
		if (a.Key == "err" || a.Key == "error") && a.Value != nil {
			if err, ok := a.Value.(error); ok {
				ev.Error = sidecarErrorFromErr(err)
				continue
			}
		}
		if _, reserved := reservedAttrKeys[a.Key]; reserved {
			switch a.Key {
			case "event":
				if s, ok := a.Value.(string); ok {
					ev.Event = s
				}
			case "trace_id":
				// ctx-bound trace_id (from WithContext) takes priority; attr
				// only fills in if ctx had nothing.
				if s, ok := a.Value.(string); ok && ev.TraceID == "" {
					ev.TraceID = s
				}
			case "span_id":
				if s, ok := a.Value.(string); ok && ev.SpanID == "" {
					ev.SpanID = s
				}
			}
			continue
		}
		ev.Attrs[a.Key] = a.Value
	}

	line, err := json.Marshal(ev)
	if err != nil {
		return nil, err
	}
	return append(line, '\n'), nil
}

func sidecarErrorFromErr(err error) *sidecarError {
	if err == nil {
		return nil
	}
	code, msg, hint := errcodeFromErr(err)
	return &sidecarError{Code: code, Message: msg, Hint: hint}
}

// jsonMarshalString is a tiny wrapper around json.Marshal for a string
// value. Exposed at package scope so trace_context.go can reuse it without
// re-importing encoding/json (keeps the import graph minimal and the JSON
// behaviour consistent between sidecar and span.end emission).
func jsonMarshalString(s string) ([]byte, error) {
	return json.Marshal(s)
}

// mergeAttrs returns the concatenation of a logger's pre-bound attrs (via
// WithAttrs) and the per-call attrs slice. The result is a new slice; inputs
// are not modified.
func mergeAttrs(bound, perCall []Attr) []Attr {
	if len(bound) == 0 {
		return perCall
	}
	if len(perCall) == 0 {
		return bound
	}
	out := make([]Attr, 0, len(bound)+len(perCall))
	out = append(out, bound...)
	out = append(out, perCall...)
	return out
}
