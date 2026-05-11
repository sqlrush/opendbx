// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package logger

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestStartSpanNilCtx(t *testing.T) {
	t.Parallel()
	// Construct nil via local var to dodge staticcheck SA1012 (which only
	// flags nil-literal context args). The contract under test is exactly
	// that StartSpan tolerates a nil context.Context value.
	var nilCtx context.Context
	ctx, sp := StartSpan(nilCtx, "boot.init")
	if ctx == nil {
		t.Fatal("StartSpan(nil) returned nil ctx")
	}
	if sp == nil {
		t.Fatal("StartSpan returned nil span")
	}
	got := spanFromContext(ctx)
	if got == nil {
		t.Fatal("spanFromContext returned nil after StartSpan")
	}
	if got.verb != "boot.init" {
		t.Errorf("span.verb = %q, want boot.init", got.verb)
	}
	if got.traceID == "" || got.spanID == "" {
		t.Errorf("trace/span IDs unset: trace=%q span=%q", got.traceID, got.spanID)
	}
	if got.parentSpanID != "" {
		t.Errorf("root span parent_span_id = %q, want empty", got.parentSpanID)
	}
}

func TestStartSpanInheritsTrace(t *testing.T) {
	t.Parallel()
	ctxA, spA := StartSpan(context.Background(), "outer")
	ctxB, spB := StartSpan(ctxA, "inner")
	defer spB.End()
	defer spA.End()

	innerImpl := spanFromContext(ctxB)
	if innerImpl == nil {
		t.Fatal("nested span not in ctxB")
	}
	outerImpl := spA.(*span)
	if innerImpl.traceID != outerImpl.traceID {
		t.Errorf("child trace_id = %q, want %q (inherited)", innerImpl.traceID, outerImpl.traceID)
	}
	if innerImpl.parentSpanID != outerImpl.spanID {
		t.Errorf("child parent_span_id = %q, want %q", innerImpl.parentSpanID, outerImpl.spanID)
	}
	if innerImpl.spanID == outerImpl.spanID {
		t.Errorf("child span_id collides with parent")
	}
}

func TestSpanSetAttrAndRecordError(t *testing.T) {
	t.Parallel()
	_, sp := StartSpan(context.Background(), "tool.call")
	sp.SetAttr("tool", "diag")
	sp.SetAttr("retries", 3)
	sp.RecordError(errors.New("boom"))
	sp.End()
	// Second End is idempotent (does not panic / re-emit).
	sp.End()
}

func TestSpanEndConcurrentIdempotent(t *testing.T) {
	t.Parallel()
	_, sp := StartSpan(context.Background(), "x")
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sp.End()
		}()
	}
	wg.Wait()
}

func TestSpanSetAttrAfterEndNoop(t *testing.T) {
	t.Parallel()
	_, sp := StartSpan(context.Background(), "x")
	sp.End()
	// Should not panic / corrupt state.
	sp.SetAttr("late", "ignored")
	sp.RecordError(errors.New("late"))
}

func TestTraceIDsFromContextNoSpan(t *testing.T) {
	t.Parallel()
	tid, sid := traceIDsFromContext(context.Background())
	if tid != "" || sid != "" {
		t.Errorf("traceIDsFromContext(no span) = (%q, %q), want empty (Q8 ★A)", tid, sid)
	}
	tid, sid = traceIDsFromContext(nil)
	if tid != "" || sid != "" {
		t.Errorf("traceIDsFromContext(nil) = (%q, %q), want empty", tid, sid)
	}
}

func TestTraceIDsFromContextWithSpan(t *testing.T) {
	t.Parallel()
	ctx, sp := StartSpan(context.Background(), "x")
	defer sp.End()
	tid, sid := traceIDsFromContext(ctx)
	spImpl := sp.(*span)
	if tid != spImpl.traceID || sid != spImpl.spanID {
		t.Errorf("traceIDsFromContext = (%q, %q), want (%q, %q)", tid, sid, spImpl.traceID, spImpl.spanID)
	}
}

// End-to-end: span.End emits a sidecar event with the expected schema fields.
func TestSpanEndEmitsSidecarEvent(t *testing.T) {
	resetForTesting(t)
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	mainLog := filepath.Join(tmp, "main.log")
	setArgvForTesting(t, "opendbx", "--debug-file", mainLog)

	if err := Init(InitInput{SessionID: "span-e2e"}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	ctx, sp := StartSpan(context.Background(), "tool.diag")
	sp.SetAttr("rows", 42)
	L().WithContext(ctx).Info("scanning", Attr{Key: "event", Value: "scan.start"})
	sp.End()
	if err := Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	sidecarPath := filepath.Join(tmp, ".opendbx", "debug", "span-e2e.events.jsonl")
	raw, err := os.ReadFile(sidecarPath)
	if err != nil {
		t.Fatalf("read sidecar: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d sidecar lines, want 2 (scan.start + span.end):\n%s", len(lines), raw)
	}

	// Line 1: scan.start with active span trace/span IDs populated.
	var scanLine sidecarEvent
	if err := json.Unmarshal([]byte(lines[0]), &scanLine); err != nil {
		t.Fatalf("parse scan line: %v", err)
	}
	if scanLine.Event != "scan.start" {
		t.Errorf("scan event = %q, want scan.start", scanLine.Event)
	}
	if scanLine.TraceID == "" || scanLine.SpanID == "" {
		t.Errorf("scan trace/span empty under active span: trace=%q span=%q", scanLine.TraceID, scanLine.SpanID)
	}

	// Line 2: span.end with duration_ms attr.
	var endLine sidecarEvent
	if err := json.Unmarshal([]byte(lines[1]), &endLine); err != nil {
		t.Fatalf("parse end line: %v", err)
	}
	if endLine.Event != "span.end" {
		t.Errorf("end event = %q, want span.end", endLine.Event)
	}
	if endLine.TraceID != scanLine.TraceID || endLine.SpanID != scanLine.SpanID {
		t.Errorf("span.end IDs do not match scan: trace(%q vs %q) span(%q vs %q)",
			endLine.TraceID, scanLine.TraceID, endLine.SpanID, scanLine.SpanID)
	}
	if _, ok := endLine.Attrs["duration_ms"]; !ok {
		t.Errorf("span.end missing duration_ms attr: %+v", endLine.Attrs)
	}
	if v, _ := endLine.Attrs["rows"].(float64); v != 42 {
		t.Errorf("span.end rows attr = %v, want 42", endLine.Attrs["rows"])
	}
}

func TestSpanEndWithErrorPopulatesErrorField(t *testing.T) {
	resetForTesting(t)
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	mainLog := filepath.Join(tmp, "main.log")
	setArgvForTesting(t, "opendbx", "--debug-file", mainLog)

	if err := Init(InitInput{SessionID: "span-err"}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	_, sp := StartSpan(context.Background(), "tool.fail")
	sp.RecordError(errors.New("boom: timeout after 30s"))
	sp.End()
	if err := Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	sidecarPath := filepath.Join(tmp, ".opendbx", "debug", "span-err.events.jsonl")
	raw, err := os.ReadFile(sidecarPath)
	if err != nil {
		t.Fatalf("read sidecar: %v", err)
	}
	got := string(raw)
	if !strings.Contains(got, `"error":{"code":"","message":"boom: timeout after 30s","hint":""}`) {
		t.Errorf("span.end error field not populated:\n%s", got)
	}
}
