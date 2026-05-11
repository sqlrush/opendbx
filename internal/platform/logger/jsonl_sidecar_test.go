// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package logger

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

func TestGetSidecarPathSessionID(t *testing.T) {
	t.Parallel()
	got := getSidecarPath("abc-123")
	if !strings.HasSuffix(got, filepath.Join("debug", "abc-123.events.jsonl")) {
		t.Errorf("sidecar path = %q, want suffix debug/abc-123.events.jsonl", got)
	}
}

func TestGetSidecarPathEmptyFallback(t *testing.T) {
	t.Parallel()
	got := getSidecarPath("")
	if !strings.HasSuffix(got, "session.events.jsonl") {
		t.Errorf("empty session sidecar path = %q, want suffix session.events.jsonl", got)
	}
}

// Q3 ★A hard constraint: sidecar path must NOT be derivable from
// `--debug-file`. We don't have direct access to a setter from getSidecarPath
// but we can verify it doesn't consult os.Args.
func TestGetSidecarPathIgnoresDebugFile(t *testing.T) {
	setArgvForTesting(t, "opendbx", "--debug-file=/tmp/main.log")
	got := getSidecarPath("sess-1")
	if strings.HasPrefix(got, "/tmp/main.log") {
		t.Errorf("sidecar path = %q, must not derive from --debug-file", got)
	}
	if !strings.HasSuffix(got, "sess-1.events.jsonl") {
		t.Errorf("sidecar path = %q, want sess-1.events.jsonl suffix", got)
	}
}

func TestMarshalSidecarEventSchemaFieldOrder(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 11, 14, 32, 1, 123_000_000, time.UTC)
	line, err := marshalSidecarEvent(
		now,
		LevelInfo,
		"llm",
		"stream delta received",
		"sess-abc",
		[]Attr{
			{Key: "event", Value: "stream.delta"},
			{Key: "chunks", Value: 3},
		},
		"trace-7", "span-7",
	)
	if err != nil {
		t.Fatalf("marshalSidecarEvent err = %v", err)
	}
	// Field order is part of the contract — verify the raw JSON layout.
	want := `{"ts":"2026-05-11T14:32:01.123Z","level":"info","module":"llm","event":"stream.delta","msg":"stream delta received","trace_id":"trace-7","span_id":"span-7","session_id":"sess-abc","attrs":{"chunks":3},"error":null}` + "\n"
	if got := string(line); got != want {
		t.Errorf("sidecar JSON field order mismatch\n  got:  %s\n  want: %s", got, want)
	}
}

// Q8 ★A: events outside an active span carry "" for trace_id / span_id.
// Schema fields remain present so jq queries don't need null checks.
func TestMarshalSidecarEventEmptyTraceIDs(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 11, 14, 32, 1, 0, time.UTC)
	line, err := marshalSidecarEvent(now, LevelInfo, "boot", "process.start", "sess-z", nil, "", "")
	if err != nil {
		t.Fatalf("marshal err = %v", err)
	}
	if !bytes.Contains(line, []byte(`"trace_id":""`)) || !bytes.Contains(line, []byte(`"span_id":""`)) {
		t.Errorf("sidecar must carry empty trace_id/span_id explicitly:\n  %s", line)
	}
}

// Reserved attr promotion: caller passes Attr{Key:"event", ...} and it lands
// in the top-level `event` field rather than the `attrs` map.
func TestMarshalSidecarEventReservedKeyPromotion(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 11, 14, 32, 1, 0, time.UTC)
	line, _ := marshalSidecarEvent(
		now, LevelInfo, "llm", "x", "sess",
		[]Attr{
			{Key: "event", Value: "tool.call"},
			{Key: "trace_id", Value: "from-attr"},
			{Key: "span_id", Value: "from-attr-span"},
			{Key: "tool", Value: "diag"},
		},
		"", "", // empty ctx → attr trace_id wins
	)
	var got sidecarEvent
	if err := json.Unmarshal(bytes.TrimRight(line, "\n"), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Event != "tool.call" {
		t.Errorf("event = %q, want tool.call", got.Event)
	}
	if got.TraceID != "from-attr" {
		t.Errorf("trace_id = %q, want from-attr (attr fills empty ctx)", got.TraceID)
	}
	if got.SpanID != "from-attr-span" {
		t.Errorf("span_id = %q, want from-attr-span", got.SpanID)
	}
	// "tool" attr stays in attrs map; reserved keys do NOT.
	if _, ok := got.Attrs["event"]; ok {
		t.Error("attrs map should not echo reserved 'event' key")
	}
	if _, ok := got.Attrs["trace_id"]; ok {
		t.Error("attrs map should not echo reserved 'trace_id' key")
	}
	if v, ok := got.Attrs["tool"]; !ok || v != "diag" {
		t.Errorf("attrs.tool = %v, want diag", v)
	}
}

// When ctx carries a non-empty trace_id, it wins over a caller-supplied
// trace_id attr. This matches T-8's eventual span propagation semantics
// (ctx is the authoritative source within a span).
func TestMarshalSidecarEventCtxTraceIDWinsOverAttr(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 11, 14, 32, 1, 0, time.UTC)
	line, _ := marshalSidecarEvent(
		now, LevelInfo, "llm", "x", "sess",
		[]Attr{{Key: "trace_id", Value: "from-attr"}},
		"from-ctx", "ctx-span",
	)
	var got sidecarEvent
	_ = json.Unmarshal(bytes.TrimRight(line, "\n"), &got)
	if got.TraceID != "from-ctx" {
		t.Errorf("trace_id = %q, want from-ctx (ctx wins over attr)", got.TraceID)
	}
	if got.SpanID != "ctx-span" {
		t.Errorf("span_id = %q, want ctx-span", got.SpanID)
	}
}

func TestMarshalSidecarEventEmptyAttrsIsObject(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 11, 14, 32, 1, 0, time.UTC)
	line, _ := marshalSidecarEvent(now, LevelInfo, "boot", "msg", "sess", nil, "", "")
	// Empty attrs map must marshal as `{}` (not `null`) so jq `.attrs.foo`
	// always returns null rather than erroring on missing object.
	if !bytes.Contains(line, []byte(`"attrs":{}`)) {
		t.Errorf("empty attrs should marshal as {}, got: %s", line)
	}
}

func TestMergeAttrs(t *testing.T) {
	t.Parallel()
	bound := []Attr{{Key: "a", Value: 1}}
	perCall := []Attr{{Key: "b", Value: 2}}

	merged := mergeAttrs(bound, perCall)
	if len(merged) != 2 || merged[0].Key != "a" || merged[1].Key != "b" {
		t.Errorf("merged = %+v, want [a=1 b=2]", merged)
	}
	// Inputs must not be mutated.
	if len(bound) != 1 || len(perCall) != 1 {
		t.Errorf("mergeAttrs mutated inputs: bound=%v perCall=%v", bound, perCall)
	}
	// Empty cases.
	if got := mergeAttrs(nil, perCall); len(got) != 1 || got[0].Key != "b" {
		t.Errorf("nil-bound merge = %v", got)
	}
	if got := mergeAttrs(bound, nil); len(got) != 1 || got[0].Key != "a" {
		t.Errorf("nil-perCall merge = %v", got)
	}
}

func TestMarshalSidecarEventPromotesErrcodeAttr(t *testing.T) {
	now := time.Date(2026, 5, 11, 14, 32, 1, 0, time.UTC)
	_ = errcode.Register("TEST.SIDECAR_ATTR_ERR", "stream failed", "retry the request")

	line, err := marshalSidecarEvent(
		now, LevelError, "llm", "stream failed", "sess",
		[]Attr{
			{Key: "event", Value: "llm.stream.error"},
			{Key: "err", Value: errcode.New("TEST.SIDECAR_ATTR_ERR", "", "")},
			{Key: "provider", Value: "anthropic"},
		},
		"", "",
	)
	if err != nil {
		t.Fatalf("marshalSidecarEvent: %v", err)
	}

	var got sidecarEvent
	if err := json.Unmarshal(bytes.TrimRight(line, "\n"), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Error == nil {
		t.Fatalf("sidecar error should be populated: %s", line)
	}
	if got.Error.Code != "TEST.SIDECAR_ATTR_ERR" ||
		got.Error.Message != "stream failed" ||
		got.Error.Hint != "retry the request" {
		t.Fatalf("sidecar error = %+v", got.Error)
	}
	if _, ok := got.Attrs["err"]; ok {
		t.Fatalf("err attr should be promoted, not echoed in attrs: %+v", got.Attrs)
	}
	if got.Attrs["provider"] != "anthropic" {
		t.Fatalf("provider attr lost: %+v", got.Attrs)
	}
}

// End-to-end: Init with sidecar enabled, emit events, Close, verify the
// JSONL file contains the events with correct schema.
func TestLoggerSidecarEndToEnd(t *testing.T) {
	resetForTesting(t)
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	mainLog := filepath.Join(tmp, "main.log")
	setArgvForTesting(t, "opendbx", "--debug-file", mainLog)

	if err := Init(InitInput{SessionID: "sidecar-e2e"}); err != nil {
		t.Fatalf("Init err = %v", err)
	}
	L().Info("api: connected", Attr{Key: "event", Value: "api.connect"})
	L().WithModule("llm").Info("stream chunk", Attr{Key: "event", Value: "llm.stream.delta"}, Attr{Key: "size", Value: 256})
	if err := Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}

	// Sidecar path: ~/.opendbx/debug/sidecar-e2e.events.jsonl
	sidecarPath := filepath.Join(tmp, ".opendbx", "debug", "sidecar-e2e.events.jsonl")
	raw, err := os.ReadFile(sidecarPath)
	if err != nil {
		t.Fatalf("read sidecar: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("sidecar lines = %d, want 2; content:\n%s", len(lines), raw)
	}

	var first sidecarEvent
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("parse line 1: %v", err)
	}
	if first.Event != "api.connect" {
		t.Errorf("first event = %q, want api.connect", first.Event)
	}
	if first.SessionID != "sidecar-e2e" {
		t.Errorf("first session_id = %q", first.SessionID)
	}
	if first.Msg != "api: connected" {
		t.Errorf("first msg = %q", first.Msg)
	}
	if first.TraceID != "" || first.SpanID != "" {
		t.Errorf("first trace/span = (%q, %q), want empty (no active span)", first.TraceID, first.SpanID)
	}

	var second sidecarEvent
	if err := json.Unmarshal([]byte(lines[1]), &second); err != nil {
		t.Fatalf("parse line 2: %v", err)
	}
	if second.Module != "llm" || second.Event != "llm.stream.delta" {
		t.Errorf("second event = (%q, %q), want (llm, llm.stream.delta)", second.Module, second.Event)
	}
	if size, _ := second.Attrs["size"].(float64); size != 256 {
		t.Errorf("second attrs.size = %v, want 256", second.Attrs["size"])
	}
}

// Q3 ★A: --debug-file redirecting main path must NOT also redirect sidecar.
// Sidecar still lands under the default debug dir.
func TestLoggerSidecarPathIndependentOfDebugFile(t *testing.T) {
	resetForTesting(t)
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	customMain := filepath.Join(tmp, "custom-main.log")
	setArgvForTesting(t, "opendbx", "--debug-file", customMain)

	if err := Init(InitInput{SessionID: "indep"}); err != nil {
		t.Fatalf("Init err = %v", err)
	}
	L().Info("ping", Attr{Key: "event", Value: "ping"})
	if err := Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}

	// Main log lives at the custom location.
	if _, err := os.Stat(customMain); err != nil {
		t.Fatalf("main log missing at %s: %v", customMain, err)
	}
	// Sidecar lives at the default location (under HOME/.opendbx/debug), NOT
	// next to or derived from customMain.
	defaultSidecar := filepath.Join(tmp, ".opendbx", "debug", "indep.events.jsonl")
	if _, err := os.Stat(defaultSidecar); err != nil {
		t.Fatalf("sidecar should live at default path %s, got: %v", defaultSidecar, err)
	}
	// And critically: NOT next to the custom main path.
	leakedSidecar := strings.TrimSuffix(customMain, ".log") + ".events.jsonl"
	if _, err := os.Stat(leakedSidecar); err == nil {
		t.Errorf("sidecar leaked to %s — should not derive from --debug-file", leakedSidecar)
	}
}

// Q9 ★A: even with --debug-to-stderr the sidecar still writes to file.
func TestLoggerSidecarFileWhenStderr(t *testing.T) {
	resetForTesting(t)
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	setArgvForTesting(t, "opendbx", "--debug-to-stderr")

	if err := Init(InitInput{SessionID: "stderr-mode"}); err != nil {
		t.Fatalf("Init err = %v", err)
	}
	L().Info("hello", Attr{Key: "event", Value: "hello"})
	if err := Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}

	sidecarPath := filepath.Join(tmp, ".opendbx", "debug", "stderr-mode.events.jsonl")
	raw, err := os.ReadFile(sidecarPath)
	if err != nil {
		t.Fatalf("sidecar must still write to file under --debug-to-stderr: %v", err)
	}
	if !strings.Contains(string(raw), `"event":"hello"`) {
		t.Errorf("sidecar content missing event: %s", raw)
	}
}

// DisableSidecar=true → no sidecar file created, main path unaffected.
func TestLoggerDisableSidecar(t *testing.T) {
	resetForTesting(t)
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	mainLog := filepath.Join(tmp, "main.log")
	setArgvForTesting(t, "opendbx", "--debug-file", mainLog)

	if err := Init(InitInput{SessionID: "no-sidecar", DisableSidecar: true}); err != nil {
		t.Fatalf("Init err = %v", err)
	}
	L().Info("ping", Attr{Key: "event", Value: "ping"})
	if err := Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}

	if _, err := os.Stat(mainLog); err != nil {
		t.Fatalf("main log missing: %v", err)
	}
	sidecarPath := filepath.Join(tmp, ".opendbx", "debug", "no-sidecar.events.jsonl")
	if _, err := os.Stat(sidecarPath); err == nil {
		t.Errorf("sidecar file %s should not exist when DisableSidecar=true", sidecarPath)
	}
}
