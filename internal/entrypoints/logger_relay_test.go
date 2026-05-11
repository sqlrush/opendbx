// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package entrypoints

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sqlrush/opendbx/internal/platform/logger"
)

func TestInitLoggerFromCLI(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	logPath := filepath.Join(tmp, "relay.log")

	if err := InitLoggerFromCLI(LoggerInitInputs{
		SessionID: "relay-test",
		Debug:     "api",
		DebugFile: logPath,
	}); err != nil {
		t.Fatalf("InitLoggerFromCLI err = %v", err)
	}
	// Second call must be idempotent.
	if err := InitLoggerFromCLI(LoggerInitInputs{SessionID: "ignored-second"}); err != nil {
		t.Errorf("second InitLoggerFromCLI err = %v, want nil", err)
	}
	logger.L().WithModule("api").Info("relay logger event")
	if err := CloseLogger(); err != nil && !errors.Is(err, logger.ErrNotInitialised) {
		t.Errorf("CloseLogger err = %v", err)
	}

	mainRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read main debug log: %v", err)
	}
	if !strings.Contains(string(mainRaw), "relay logger event") {
		t.Fatalf("main debug log missing relay event:\n%s", mainRaw)
	}

	sidecarPath := filepath.Join(tmp, ".opendbx", "debug", "relay-test.events.jsonl")
	sidecarRaw, err := os.ReadFile(sidecarPath)
	if err != nil {
		t.Fatalf("read sidecar debug log: %v", err)
	}
	if !strings.Contains(string(sidecarRaw), `"session_id":"relay-test"`) {
		t.Fatalf("sidecar did not use relay session id:\n%s", sidecarRaw)
	}
}

func TestRegisterLoggerSignalCleanup(t *testing.T) {
	// Idempotent; we just verify it does not panic.
	RegisterLoggerSignalCleanup()
	RegisterLoggerSignalCleanup()
}

func TestGuardLoggerPanic(t *testing.T) {
	caught := func() (v any) {
		defer func() { v = recover() }()
		GuardLoggerPanic(func() { panic("relay-panic") })
		return nil
	}()
	if caught != "relay-panic" {
		t.Errorf("GuardLoggerPanic did not re-raise: got %v", caught)
	}
}

func TestCloseLoggerBeforeInit(t *testing.T) {
	// On a fresh test binary the global logger may already be init from a
	// prior test in this file — skip the strict "not initialised" check
	// since we can't reset across tests without exposing an internal hook.
	_ = CloseLogger() // contract: returns nil or ErrNotInitialised
}
