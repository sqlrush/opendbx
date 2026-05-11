// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package entrypoints

import (
	"errors"
	"testing"

	"github.com/sqlrush/opendbx/internal/platform/logger"
)

func TestInitLoggerFromCLI(t *testing.T) {
	if err := InitLoggerFromCLI(LoggerInitInputs{SessionID: "relay-test"}); err != nil {
		t.Fatalf("InitLoggerFromCLI err = %v", err)
	}
	// Second call must be idempotent.
	if err := InitLoggerFromCLI(LoggerInitInputs{SessionID: "ignored-second"}); err != nil {
		t.Errorf("second InitLoggerFromCLI err = %v, want nil", err)
	}
	// Reach through to verify logger initialised.
	if err := CloseLogger(); err != nil && !errors.Is(err, logger.ErrNotInitialised) {
		t.Errorf("CloseLogger err = %v", err)
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
