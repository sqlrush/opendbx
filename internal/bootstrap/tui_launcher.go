// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package bootstrap

import (
	"context"
	"errors"
	"sync"

	"github.com/gdamore/tcell/v2"

	"github.com/sqlrush/opendbx/internal/app/cli/llmapp"
	"github.com/sqlrush/opendbx/internal/app/cli/program"
	tcelladapter "github.com/sqlrush/opendbx/internal/app/cli/render/terminal/tcell"
	"github.com/sqlrush/opendbx/internal/app/cli/tui"
	"github.com/sqlrush/opendbx/internal/domain/llm"
	"github.com/sqlrush/opendbx/internal/domain/llm/factory"
	"github.com/sqlrush/opendbx/internal/domain/llm/fake"
	"github.com/sqlrush/opendbx/internal/platform/config"
)

// newScreenFn is the screen factory for the program.Run production path
// (spec-1.17 D-6b). It returns an UN-init'd screen — the tcell adapter's
// Driver.Init (invoked by scheduler.Run) owns screen.Init. Production
// uses tui.NewScreenNoInit; tests replace it with a SimulationScreen
// factory (SimulationScreen also returns un-init'd, matching the
// contract; Driver.Init runs sim.Init()).
//
// T-13 go M-2: newScreenFnMu guards mutation/read in case tests run in
// parallel inside this package. Current tests document "NOT t.Parallel"
// but the mutex makes the contract machine-enforced.
//
//nolint:gochecknoglobals // spec-0.12 D-3 / spec-1.17 D-6b: test seam for SimulationScreen injection.
var (
	newScreenFnMu sync.RWMutex
	newScreenFn   = tui.NewScreenNoInit
)

// getNewScreenFn returns the current factory (read-locked).
func getNewScreenFn() func() (tcell.Screen, error) {
	newScreenFnMu.RLock()
	defer newScreenFnMu.RUnlock()
	return newScreenFn
}

// setNewScreenFn replaces the factory (write-locked). Tests call this
// via the test-only setNewScreenFn function exported within the same
// package (see tui_launcher_test.go).
func setNewScreenFn(fn func() (tcell.Screen, error)) {
	newScreenFnMu.Lock()
	defer newScreenFnMu.Unlock()
	newScreenFn = fn
}

// LaunchInteractiveTUI runs the production interactive TUI via the
// spec-1.15 Program main loop (spec-1.17 D-6b; spec-1.15 Q13 ★A
// retroactive impl). Layer chain (spec-0.12 D-4):
//
//	cmd → entrypoints → bootstrap → app/cli/{program,demoapp,render/terminal/tcell}
//
// Lifecycle (spec-1.17 D-6a): the screen is constructed un-init'd; the
// tcell adapter's Driver.Init (called inside scheduler.Run) owns
// screen.Init, and Driver.Fini owns screen.Fini (sync.Once idempotent).
// bootstrap does NOT defer screen.Fini — that would double-Fini.
//
// Returns nil on key-exit (Ctrl+C double-press / Ctrl+\), ctx.Err on
// cancel, ErrInitFailed wrap on tcell screen construction failure.
//
// The Model is llmapp.New (spec-1.20 D-6 production chat). 原则 3: when
// the LLM provider cannot be constructed (no API key / unknown provider),
// we do NOT silently fall back to demoapp — instead llmapp is started with
// a provider that surfaces the LLM.* errcode on the first message, so the
// user sees an explicit, actionable error.
func LaunchInteractiveTUI(ctx context.Context) error {
	screen, err := getNewScreenFn()()
	if err != nil {
		// errcode-lint:exempt -- spec-0.12 D-3: err is already wrapped as TERMINAL.INIT_FAILED by tui.NewScreenNoInit; pass-through.
		return err
	}
	// NOTE: no `defer screen.Fini()` — Driver.Fini (spec-1.17 D-6a)
	// owns screen teardown via scheduler.Run shutdown.
	driver := tcelladapter.NewDriver(screen)
	model := newChatModel()
	p := program.New(driver, model)

	// errcode-lint:exempt -- spec-1.17 D-6b: program.Run returns scheduler.Run's result verbatim (context.Canceled / DeadlineExceeded on shutdown; pre-wrapped driver.Init errors otherwise). The mapping below converts an internal user-quit cancel to nil; all returned errors are stdlib sentinels or pre-wrapped.
	runErr := p.Run(ctx)

	// spec-1.17 D-6b: program.Run cancels an internal child context on
	// the user quit protocol (Ctrl+C double-press / Ctrl+\), so it
	// returns context.Canceled even on a clean key-exit. Distinguish:
	// if the PARENT ctx was cancelled the cancellation is external —
	// pass the error through; otherwise it was a user quit — return nil
	// to preserve the spec-0.12 "Ctrl+C exits cleanly" contract.
	if ctx.Err() != nil {
		return runErr
	}
	if errors.Is(runErr, context.Canceled) {
		return nil
	}
	return runErr
}

// newChatModel loads config, builds the LLM provider via the factory, and
// returns the spec-1.20 llmapp chat Model. 原则 3: a provider-construction
// failure does NOT fall back to demoapp — it yields a provider whose
// Stream returns the LLM.* errcode, so the user gets an explicit error
// (with the actionable Hint) on their first message rather than a silent
// offline demo.
func newChatModel() program.Model {
	cfg, cfgErr := config.Load(config.LoadOptions{})
	if cfgErr != nil || cfg == nil {
		// Config load failed entirely — surface UNAVAILABLE on first message.
		return llmapp.New(fake.New().WithStartErr(llm.ErrUnavailable), llmapp.Options{})
	}
	provider, perr := factory.New(*cfg)
	opts := llmapp.Options{
		ModelName:      cfg.LLM.ActiveModel,
		MaxHistory:     cfg.Session.MaxHistoryMessages,
		StripThink:     cfg.LLM.StripThink,
		ThinkingMode:   thinkingModeFromConfig(cfg.LLM.ThinkingMode),
		ThinkingBudget: cfg.LLM.ThinkingBudget,
	}
	if perr != nil {
		// 原则 3: explicit error, no demoapp fallback.
		return llmapp.New(fake.New().WithStartErr(perr), opts)
	}
	return llmapp.New(provider, opts)
}

// thinkingModeFromConfig maps the config thinking_mode string to the
// domain enum (T-10a HIGH-2 wiring). "adaptive" is never constructed here —
// factory.New rejects it with LLM.NOT_IMPLEMENTED before this is reached —
// so it folds into Disabled defensively.
func thinkingModeFromConfig(s string) llm.ThinkingMode {
	if s == "enabled" {
		return llm.ThinkingEnabled
	}
	return llm.ThinkingDisabled
}
