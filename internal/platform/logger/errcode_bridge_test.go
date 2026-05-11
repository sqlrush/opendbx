// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package logger

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

// register a TEST.* code local to this test file for the 4-form coverage.
func registerLoggerTestCode(t *testing.T, code, msg, hint string) errcode.Sentinel {
	t.Helper()
	s := errcode.Register(code, msg, hint)
	t.Cleanup(func() {
		// The errcode package exposes unregisterForTesting at file scope but
		// from inside its own package; we cannot reach it here. The TEST.*
		// prefix excludes from All(), so leaking is harmless for the test
		// process lifetime.
		_ = s
	})
	return s
}

func TestErrcodeFromErrPlain(t *testing.T) {
	t.Parallel()
	registerLoggerTestCode(t, "TEST.PLAIN_ERRCODE", "msg", "hint")
	ec := errcode.New("TEST.PLAIN_ERRCODE", "", "")
	code, msg, hint := errcodeFromErr(ec)
	if code != "TEST.PLAIN_ERRCODE" || msg != "msg" || hint != "hint" {
		t.Errorf("plain errcode: (%q,%q,%q), want (TEST.PLAIN_ERRCODE,msg,hint)", code, msg, hint)
	}
}

func TestErrcodeFromErrFmtErrorfWrapped(t *testing.T) {
	t.Parallel()
	registerLoggerTestCode(t, "TEST.FMTWRAP_INNER", "inner msg", "inner hint")
	inner := errcode.New("TEST.FMTWRAP_INNER", "", "")
	outer := fmt.Errorf("operation failed: %w", inner)
	code, msg, hint := errcodeFromErr(outer)
	if code != "TEST.FMTWRAP_INNER" || msg != "inner msg" || hint != "inner hint" {
		t.Errorf("fmt.Errorf wrap: (%q,%q,%q)", code, msg, hint)
	}
}

func TestErrcodeFromErrErrcodeWrap(t *testing.T) {
	t.Parallel()
	registerLoggerTestCode(t, "TEST.ECWRAP_INNER", "inner", "ih")
	registerLoggerTestCode(t, "TEST.ECWRAP_OUTER", "outer", "oh")
	inner := errcode.New("TEST.ECWRAP_INNER", "", "")
	outer := errcode.Wrap("TEST.ECWRAP_OUTER", inner, "", "")
	code, msg, hint := errcodeFromErr(outer)
	// errors.As binds to the outermost errcode.Error in the chain.
	if code != "TEST.ECWRAP_OUTER" || msg != "outer" || hint != "oh" {
		t.Errorf("errcode.Wrap: (%q,%q,%q), want outer", code, msg, hint)
	}
	// Stdlib errors.Is can still see the inner via chain.
	if !errors.Is(outer, inner) {
		t.Error("errors.Is(outer, inner) should walk chain")
	}
}

func TestErrcodeFromErrRedactedErrorWrapped(t *testing.T) {
	t.Parallel()
	registerLoggerTestCode(t, "TEST.REDACT_INNER", "secret context", "fix it")
	inner := errcode.New("TEST.REDACT_INNER", "", "")
	wrapped := redactedError{msg: "redacted layer", wrapped: inner}
	code, msg, hint := errcodeFromErr(wrapped)
	if code != "TEST.REDACT_INNER" {
		t.Errorf("redactedError chain: code = %q, want TEST.REDACT_INNER", code)
	}
	if msg != "secret context" || hint != "fix it" {
		t.Errorf("redactedError chain: msg/hint = (%q,%q)", msg, hint)
	}
}

func TestErrcodeFromErrNonErrcode(t *testing.T) {
	t.Parallel()
	plain := errors.New("not a structured error")
	code, msg, hint := errcodeFromErr(plain)
	if code != "" || hint != "" {
		t.Errorf("non-errcode fallback: code/hint should be empty, got (%q,%q)", code, hint)
	}
	if msg != "not a structured error" {
		t.Errorf("non-errcode fallback: msg = %q", msg)
	}
}

func TestErrcodeFromErrNil(t *testing.T) {
	t.Parallel()
	code, msg, hint := errcodeFromErr(nil)
	if code != "" || msg != "" || hint != "" {
		t.Errorf("nil err should yield empty triple, got (%q,%q,%q)", code, msg, hint)
	}
}

// End-to-end: emit a span with an errcode.Error attached via RecordError;
// verify the sidecar JSONL contains the full Code/Message/Hint.
func TestSpanEndWithErrcodeSidecarFull(t *testing.T) {
	resetForTesting(t)
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	mainLog := filepath.Join(tmp, "main.log")
	setArgvForTesting(t, "opendbx", "--debug", "--debug-file", mainLog)

	registerLoggerTestCode(t, "TEST.SPAN_ERRCODE_E2E",
		"operation timed out",
		"increase OPENDBX_LLM_REQUEST_TIMEOUT or check network")

	if err := Init(InitInput{SessionID: "errcode-e2e"}); err != nil {
		t.Fatalf("Init: %v", err)
	}

	_, sp := StartSpan(context.Background(), "tool.run")
	ecErr := errcode.New("TEST.SPAN_ERRCODE_E2E", "", "")
	sp.RecordError(ecErr)
	sp.End()
	if err := Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	sidecar := filepath.Join(tmp, ".opendbx", "debug", "errcode-e2e.events.jsonl")
	raw, err := os.ReadFile(sidecar)
	if err != nil {
		t.Fatalf("sidecar: %v", err)
	}
	got := string(raw)

	// All three sidecar error fields populated.
	wants := []string{
		`"code":"TEST.SPAN_ERRCODE_E2E"`,
		`"message":"operation timed out"`,
		`"hint":"increase OPENDBX_LLM_REQUEST_TIMEOUT or check network"`,
	}
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("sidecar missing %q:\n%s", w, got)
		}
	}
}

// End-to-end: fmt.Errorf wrap of errcode still surfaces structured fields.
func TestSpanEndWithFmtWrappedErrcodeSidecar(t *testing.T) {
	resetForTesting(t)
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	mainLog := filepath.Join(tmp, "main.log")
	setArgvForTesting(t, "opendbx", "--debug", "--debug-file", mainLog)

	registerLoggerTestCode(t, "TEST.FMTWRAP_E2E", "stream stalled", "check provider status")

	if err := Init(InitInput{SessionID: "fmtwrap-e2e"}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	_, sp := StartSpan(context.Background(), "tool.run")
	inner := errcode.New("TEST.FMTWRAP_E2E", "", "")
	wrapped := fmt.Errorf("higher context: %w", inner)
	sp.RecordError(wrapped)
	sp.End()
	if err := Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(tmp, ".opendbx", "debug", "fmtwrap-e2e.events.jsonl"))
	if err != nil {
		t.Fatalf("sidecar: %v", err)
	}
	got := string(raw)
	if !strings.Contains(got, `"code":"TEST.FMTWRAP_E2E"`) {
		t.Errorf("fmt.Errorf wrap did not surface inner code:\n%s", got)
	}
}
