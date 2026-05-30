// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package bootstrap

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	tcellpkg "github.com/sqlrush/opendbx/internal/app/cli/tui"
	"github.com/sqlrush/opendbx/internal/domain/llm"
	"github.com/sqlrush/opendbx/internal/platform/config"
	"github.com/sqlrush/opendbx/internal/platform/logger"
)

func initLoggerForTUITest(t *testing.T) {
	t.Helper()
	err := logger.Init(logger.InitInput{
		SessionID:      "bootstrap-test",
		LogPath:        filepath.Join(t.TempDir(), "debug.log"),
		DisableSidecar: true,
	})
	if err != nil && !errors.Is(err, logger.ErrAlreadyInitialised) {
		t.Fatalf("logger.Init: %v", err)
	}
}

// TestLaunchInteractiveTUI_NewScreenFailure exercises the init-failure
// path. Replaces the screen factory with a stub that always errors.
// spec-1.17 D-6b: error pass-through unchanged from spec-0.12.
func TestLaunchInteractiveTUI_NewScreenFailure(t *testing.T) {
	initLoggerForTUITest(t)
	// NOT t.Parallel — mutates package-global factory state.
	orig := getNewScreenFn()
	setNewScreenFn(func() (tcell.Screen, error) {
		return nil, tcellpkg.ErrInitFailed
	})
	t.Cleanup(func() { setNewScreenFn(orig) })

	err := LaunchInteractiveTUI(context.Background())
	if !errors.Is(err, tcellpkg.ErrInitFailed) {
		t.Errorf("expected ErrInitFailed; got %v", err)
	}
}

// TestLaunchInteractiveTUI_HappyPath covers the program.Run production
// path (spec-1.17 D-6b migration from the legacy tui.Run path). The
// factory returns an un-init'd SimulationScreen; the tcell adapter's
// Driver.Init (driven by scheduler.Run) initializes it. A Ctrl+C
// double-press drives the quit protocol so Run returns nil.
//
// spec-1.17 R-6 absorb: this test was migrated from tui.Run +
// SimulationScreen to program.Run + SimulationScreen. The legacy
// tui.Run path is retired from production (no caller) but its function
// body remains for any future legacy use.
func TestLaunchInteractiveTUI_HappyPath(t *testing.T) {
	initLoggerForTUITest(t)
	// NOT t.Parallel — mutates package-global factory state.
	orig := getNewScreenFn()
	sim := tcellpkg.NewSimulationScreen()
	// NOTE: do NOT Init the sim here — the tcell adapter's Driver.Init
	// (invoked by scheduler.Run) owns Init (spec-1.17 D-6a lifecycle).
	setNewScreenFn(func() (tcell.Screen, error) {
		return sim, nil
	})
	t.Cleanup(func() { setNewScreenFn(orig) })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Drive a Ctrl+C double-press once the screen is live. The program
	// quit protocol (spec-1.15 D-5) treats two Ctrl+C within the window
	// as quit → program.Run returns nil.
	go func() {
		time.Sleep(60 * time.Millisecond)
		sim.InjectKey(tcell.KeyCtrlC, 0, tcell.ModCtrl)
		time.Sleep(20 * time.Millisecond)
		sim.InjectKey(tcell.KeyCtrlC, 0, tcell.ModCtrl)
	}()

	if err := LaunchInteractiveTUI(ctx); err != nil {
		t.Errorf("expected nil from Ctrl+C double-press quit; got %v", err)
	}
}

// TestLaunchInteractiveTUI_RestoresSlogDefaultOnFailure guards the TUI slog
// bridge lifecycle: LaunchInteractiveTUI installs logger.NewSlogHandler while
// it owns the terminal, then restores the previous stdlib slog default even on
// early screen-construction failure.
func TestLaunchInteractiveTUI_RestoresSlogDefaultOnFailure(t *testing.T) {
	initLoggerForTUITest(t)
	// NOT t.Parallel: mutates package-global factory state and slog default.
	orig := getNewScreenFn()
	setNewScreenFn(func() (tcell.Screen, error) {
		return nil, tcellpkg.ErrInitFailed
	})
	t.Cleanup(func() { setNewScreenFn(orig) })

	prev := slog.Default()
	if err := LaunchInteractiveTUI(context.Background()); !errors.Is(err, tcellpkg.ErrInitFailed) {
		t.Fatalf("LaunchInteractiveTUI err = %v, want ErrInitFailed", err)
	}
	if slog.Default() != prev {
		t.Fatal("LaunchInteractiveTUI did not restore slog default after failure")
	}
}

// TestDefaultDiagnoseRegistry_HasClockAndEcho guards the spec-1.21 T-9
// boundary: bootstrap MUST inject a Registry containing the minimal
// read-only tools so real interact sessions can execute clock/echo
// round-trips. A future spec adding production skills should NOT
// silently drop these; updates are additive only.
func TestDefaultDiagnoseRegistry_HasClockAndEcho(t *testing.T) {
	t.Parallel()
	reg := defaultDiagnoseRegistry()
	if reg == nil {
		t.Fatal("defaultDiagnoseRegistry returned nil")
	}
	names := reg.Names()
	want := map[string]bool{"clock": true, "echo": true}
	for _, n := range names {
		delete(want, n)
	}
	if len(want) > 0 {
		t.Errorf("registry missing tools: %v (got %v)", want, names)
	}
}

// TestNewChatModel_PropagatesDiagnoseConfig is the user T-10a Path 3/3
// HIGH-1 absorb: spec-1.21 D-6 mandates that DiagnoseConfig values
// (MaxTurns / ToolTimeout / TotalTimeout) AND the per-turn
// LLMConfig.RequestTimeout reach the diagnose.Loop via llmapp.Options.
// We can't introspect the Loop's internal timers from here, so the
// test asserts the bootstrap path resolves the config knobs without
// panicking under non-default values + non-default cross-field-valid
// combos. The unit tests in internal/app/diagnose cover Loop's actual
// timeout behavior; this test guards against the wiring breaking.
func TestNewChatModel_PropagatesDiagnoseConfig(t *testing.T) {
	// NOT t.Parallel: env Setenv must not race other tests.
	t.Setenv("OPENDBX_DIAGNOSE_MAX_TURNS", "8")
	t.Setenv("OPENDBX_DIAGNOSE_TOOL_TIMEOUT", "45s")
	t.Setenv("OPENDBX_DIAGNOSE_TOTAL_TIMEOUT", "5m")
	t.Setenv("OPENDBX_LLM_REQUEST_TIMEOUT", "90s")
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("newChatModel panicked under env-overridden Diagnose+RequestTimeout: %v", r)
		}
	}()
	if m := newChatModel(); m == nil {
		t.Error("newChatModel returned nil under env-overridden Diagnose config")
	}
}

// TestNewChatModel_BuildsWithoutPanic exercises the production wiring
// path end-to-end at the bootstrap level so a future regression that
// breaks the Registry → Options → diagnose.NewLoop chain surfaces here
// (rather than at first user keystroke).
func TestNewChatModel_BuildsWithoutPanic(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("newChatModel panicked: %v", r)
		}
	}()
	if m := newChatModel(); m == nil {
		t.Error("newChatModel returned nil")
	}
}

// TestThinkingModeFromConfig covers the T-10a HIGH-2 config→domain mapping.
func TestThinkingModeFromConfig(t *testing.T) {
	t.Parallel()
	cases := map[string]llm.ThinkingMode{
		"enabled":  llm.ThinkingEnabled,
		"disabled": llm.ThinkingDisabled,
		"adaptive": llm.ThinkingDisabled, // factory rejects adaptive before this
		"":         llm.ThinkingDisabled,
	}
	for in, want := range cases {
		if got := thinkingModeFromConfig(in); got != want {
			t.Errorf("thinkingModeFromConfig(%q) = %v; want %v", in, got, want)
		}
	}
}

// ============================================================
// spec-1.20.2 D-5: StripThink BREAKING migration notice
// ============================================================

// captureMigrationEmit installs a stripThinkMigrationLogFn that records
// every call; returns the slice + a restore func. Bypasses the platform
// logger singleton's init/close ordering, which is shared across tests
// in the same process and fights non-parallel test seams.
func captureMigrationEmit(t *testing.T) (*[]string, func()) {
	t.Helper()
	prev := stripThinkMigrationLogFn
	var calls []string
	stripThinkMigrationLogFn = func(msg string, _ ...any) {
		calls = append(calls, msg)
	}
	return &calls, func() { stripThinkMigrationLogFn = prev }
}

// TestEmitStripThinkMigrationNotice_DefaultTrue verifies the one-shot
// migration log fires when StripThink got the new default and the
// config Source is SourceDefault.
func TestEmitStripThinkMigrationNotice_DefaultTrue(t *testing.T) {
	// NOT t.Parallel: mutates once latch + log function pointer.
	resetStripThinkMigrationNoticeForTest()
	calls, restore := captureMigrationEmit(t)
	defer restore()

	cfg := config.Default() // StripThink=true, Source=SourceDefault
	emitStripThinkMigrationNotice(cfg)

	if len(*calls) != 1 {
		t.Fatalf("emit count = %d; want 1", len(*calls))
	}
	msg := (*calls)[0]
	if !strings.Contains(msg, "spec-1.20.2") || !strings.Contains(msg, "strip_think") {
		t.Errorf("migration notice missing markers: %q", msg)
	}
}

// TestEmitStripThinkMigrationNotice_ExplicitTrueSuppressed verifies
// that an operator who explicitly set strip_think: true via yaml / env
// does NOT see the migration notice (their setting is intentional, so
// the notice would be noise).
func TestEmitStripThinkMigrationNotice_ExplicitTrueSuppressed(t *testing.T) {
	// NOT t.Parallel: mutates once latch + log function pointer.
	resetStripThinkMigrationNoticeForTest()
	calls, restore := captureMigrationEmit(t)
	defer restore()

	cfg := config.Default()
	cfg.SetSource("LLM.StripThink", config.SourceUserSettings)
	emitStripThinkMigrationNotice(cfg)

	if len(*calls) != 0 {
		t.Errorf("emit fired despite explicit user source: %v", *calls)
	}
}

// TestEmitStripThinkMigrationNotice_FalseSuppressed verifies that when
// StripThink is false (legacy behavior preserved by explicit opt-out),
// no notice fires regardless of source.
func TestEmitStripThinkMigrationNotice_FalseSuppressed(t *testing.T) {
	// NOT t.Parallel: mutates once latch + log function pointer.
	resetStripThinkMigrationNoticeForTest()
	calls, restore := captureMigrationEmit(t)
	defer restore()

	cfg := config.Default()
	cfg.LLM.StripThink = false
	cfg.SetSource("LLM.StripThink", config.SourceUserSettings)
	emitStripThinkMigrationNotice(cfg)

	if len(*calls) != 0 {
		t.Errorf("emit fired despite strip_think=false: %v", *calls)
	}
}

// TestEmitStripThinkMigrationNotice_OnceLatch verifies that two calls
// in the same process produce exactly one log record (the latch is
// process-wide so newChatModel re-entry does not spam).
func TestEmitStripThinkMigrationNotice_OnceLatch(t *testing.T) {
	// NOT t.Parallel: mutates once latch + log function pointer.
	resetStripThinkMigrationNoticeForTest()
	calls, restore := captureMigrationEmit(t)
	defer restore()

	cfg := config.Default()
	emitStripThinkMigrationNotice(cfg)
	emitStripThinkMigrationNotice(cfg)

	if len(*calls) != 1 {
		t.Errorf("emit fired %d times; want exactly 1 (sync.Once)", len(*calls))
	}
}

// TestEmitStripThinkMigrationNotice_NilConfigSafe guards against the
// degenerate path where caller passes nil — must not panic.
func TestEmitStripThinkMigrationNotice_NilConfigSafe(t *testing.T) {
	// NOT t.Parallel: mutates once latch.
	resetStripThinkMigrationNoticeForTest()
	_, restore := captureMigrationEmit(t)
	defer restore()
	emitStripThinkMigrationNotice(nil) // must not panic
}
