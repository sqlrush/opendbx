// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package logger

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// WarnForceFile writes a warning directly to the active debug file even when
// debug logging is disabled. It never writes to stderr and no-ops before Init.
func WarnForceFile(msg string, kv ...any) {
	forceFile(LevelWarn, msg, attrsFromKV(kv...))
}

// ErrorForceFile writes an error directly to the active debug file even when
// debug logging is disabled. It never writes to stderr and no-ops before Init.
func ErrorForceFile(msg string, kv ...any) {
	forceFile(LevelError, msg, attrsFromKV(kv...))
}

// NewSlogHandler returns a slog.Handler that routes stdlib slog records into
// the platform logger's debug file only. All levels bypass the normal logger
// writer so TUI-mode diagnostics are preserved without writing to stderr,
// even when the process was launched with --debug-to-stderr.
//
// Records seen before logger.Init are discarded. If the same handler later
// observes an initialised logger, it emits one force-file warning recording the
// number of discarded startup records; it never buffers and replays them.
func NewSlogHandler() slog.Handler {
	return &slogHandler{
		droppedBeforeInit: &atomic.Int64{},
		warnDroppedOnce:   &sync.Once{},
	}
}

type slogHandler struct {
	attrs  []slog.Attr
	groups []string

	droppedBeforeInit *atomic.Int64
	warnDroppedOnce   *sync.Once
}

func (h *slogHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h *slogHandler) Handle(_ context.Context, rec slog.Record) error {
	impl := current.Load()
	if impl == nil {
		h.droppedBeforeInit.Add(1)
		return nil
	}
	h.warnIfStartupDropped()

	attrs := h.recordAttrs(rec)
	msg := rec.Message
	switch {
	case rec.Level >= slog.LevelError:
		forceFileAttrs(LevelError, msg, attrs)
	case rec.Level >= slog.LevelWarn:
		forceFileAttrs(LevelWarn, msg, attrs)
	case rec.Level >= slog.LevelInfo:
		fileOnlyAttrs(LevelInfo, msg, attrs)
	default:
		fileOnlyAttrs(LevelDebug, msg, attrs)
	}
	return nil
}

func (h *slogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := h.clone()
	clone.attrs = append(clone.attrs, attrs...)
	return clone
}

func (h *slogHandler) WithGroup(name string) slog.Handler {
	if strings.TrimSpace(name) == "" {
		return h
	}
	clone := h.clone()
	clone.groups = append(clone.groups, name)
	return clone
}

func (h *slogHandler) clone() *slogHandler {
	return &slogHandler{
		attrs:             append([]slog.Attr(nil), h.attrs...),
		groups:            append([]string(nil), h.groups...),
		droppedBeforeInit: h.droppedBeforeInit,
		warnDroppedOnce:   h.warnDroppedOnce,
	}
}

func (h *slogHandler) warnIfStartupDropped() {
	if h.droppedBeforeInit.Load() == 0 {
		return
	}
	h.warnDroppedOnce.Do(func() {
		forceFileAttrs(LevelWarn, "slog records discarded before logger.Init", []Attr{{
			Key:   "count",
			Value: h.droppedBeforeInit.Load(),
		}})
	})
}

func (h *slogHandler) recordAttrs(rec slog.Record) []Attr {
	attrs := make([]Attr, 0, len(h.attrs)+rec.NumAttrs())
	for _, a := range h.attrs {
		attrs = append(attrs, h.convertAttr(a))
	}
	rec.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, h.convertAttr(a))
		return true
	})
	return attrs
}

func (h *slogHandler) convertAttr(a slog.Attr) Attr {
	a.Value = a.Value.Resolve()
	key := a.Key
	if len(h.groups) > 0 {
		key = strings.Join(append(append([]string(nil), h.groups...), key), ".")
	}
	return Attr{Key: key, Value: slogValue(a.Value)}
}

func slogValue(v slog.Value) any {
	switch v.Kind() {
	case slog.KindGroup:
		group := v.Group()
		out := make(map[string]any, len(group))
		for _, a := range group {
			a.Value = a.Value.Resolve()
			out[a.Key] = slogValue(a.Value)
		}
		return out
	case slog.KindLogValuer:
		return slogValue(v.Resolve())
	default:
		return v.Any()
	}
}

func attrsFromKV(kv ...any) []Attr {
	attrs := make([]Attr, 0, len(kv)/2)
	for i := 0; i < len(kv); i++ {
		switch v := kv[i].(type) {
		case Attr:
			attrs = append(attrs, v)
		case slog.Attr:
			attrs = append(attrs, Attr{Key: v.Key, Value: slogValue(v.Value.Resolve())})
		case string:
			if i+1 >= len(kv) {
				attrs = append(attrs, Attr{Key: v, Value: "<missing>"})
				continue
			}
			attrs = append(attrs, Attr{Key: v, Value: kv[i+1]})
			i++
		default:
			attrs = append(attrs, Attr{Key: fmt.Sprintf("arg%d", i), Value: v})
		}
	}
	return attrs
}

func forceFile(level Level, msg string, attrs []Attr) {
	forceFileAttrs(level, msg, attrs)
}

func forceFileAttrs(level Level, msg string, attrs []Attr) {
	if level < LevelWarn {
		level = LevelWarn
	}
	fileOnlyAttrs(level, msg, attrs)
}

func fileOnlyAttrs(level Level, msg string, attrs []Attr) {
	impl := current.Load()
	if impl == nil {
		return
	}
	impl.fileOnly(level, msg, attrs)
}

func (l *loggerImpl) fileOnly(level Level, msg string, attrs []Attr) {
	merged := redactAttrs(mergeAttrs(l.attrs, attrs))
	line := redactString(formatEvent(time.Now(), level, formatForceMessage(redactString(msg), merged)))
	_ = mainWriteFunc(l.logPath, false)(line)
}

func formatForceMessage(msg string, attrs []Attr) string {
	if len(attrs) == 0 {
		return msg
	}
	var b strings.Builder
	b.Grow(len(msg) + len(attrs)*16)
	b.WriteString(msg)
	for _, a := range attrs {
		if strings.TrimSpace(a.Key) == "" {
			continue
		}
		b.WriteByte(' ')
		b.WriteString(a.Key)
		b.WriteByte('=')
		b.WriteString(fmt.Sprint(a.Value))
	}
	return b.String()
}
