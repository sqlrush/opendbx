// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package bootstrap

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"

	"github.com/gdamore/tcell/v2"

	"github.com/sqlrush/opendbx/internal/app/cli/llmapp"
	"github.com/sqlrush/opendbx/internal/app/cli/program"
	tcelladapter "github.com/sqlrush/opendbx/internal/app/cli/render/terminal/tcell"
	"github.com/sqlrush/opendbx/internal/app/cli/tui"
	"github.com/sqlrush/opendbx/internal/app/diagnose"
	"github.com/sqlrush/opendbx/internal/domain/llm"
	"github.com/sqlrush/opendbx/internal/domain/llm/factory"
	"github.com/sqlrush/opendbx/internal/domain/llm/fake"
	"github.com/sqlrush/opendbx/internal/platform/config"
	"github.com/sqlrush/opendbx/internal/platform/logger"
	"github.com/sqlrush/opendbx/internal/platform/version"
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
// The Model is llmapp.New (spec-1.20 D-6 production chat). Principle 3: when
// the LLM provider cannot be constructed (no API key / unknown provider),
// we do NOT silently fall back to demoapp — instead llmapp is started with
// a provider that surfaces the LLM.* errcode on the first message, so the
// user sees an explicit, actionable error.
func LaunchInteractiveTUI(ctx context.Context) error {
	if !logger.IsInitialised() {
		return logger.ErrNotInitialised
	}

	// Route stdlib slog into the platform logger while TUI owns stderr.
	// scheduler/render packages use slog.Warn/Error for frame-budget and
	// channel-saturation diagnostics; in an interactive terminal those records
	// must be preserved in the debug file without tearing the cell grid.
	prevSlog := slog.Default()
	slog.SetDefault(slog.New(logger.NewSlogHandler()))
	defer slog.SetDefault(prevSlog)

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
// returns the spec-1.20 llmapp chat Model. Principle 3: a provider-construction
// failure does NOT fall back to demoapp — it yields a provider whose
// Stream returns the LLM.* errcode, so the user gets an explicit error
// (with the actionable Hint) on their first message rather than a silent
// offline demo.
func newChatModel() program.Model {
	cfg, cfgErr := config.Load(config.LoadOptions{})
	if cfgErr != nil || cfg == nil {
		// Config load failed entirely — surface UNAVAILABLE on first message.
		return llmapp.New(fake.New().WithStartErr(llm.ErrUnavailable), llmapp.Options{Registry: defaultDiagnoseRegistry()})
	}
	emitStripThinkMigrationNotice(cfg)
	provider, perr := factory.New(*cfg)
	// spec-2.3 D-5: one-shot skill discovery → SkillTool + system-prompt
	// section. Production's FIRST SystemPrompt assignment — the request
	// transitions empty→non-empty when skills are present (Q10). Failures
	// log + skip; interact always starts (user decision 4/4 — no panic).
	// Skipped on the provider-error path: the session cannot chat, so the
	// filesystem scan would be wasted I/O (post-impl cr LOW-1).
	var execs []diagnose.ToolExecutor
	var skillPrompt string
	if perr == nil {
		var skillExecs []diagnose.ToolExecutor
		skillExecs, skillPrompt = skillsForChat(DiscoverSkills(cfg))
		execs = append(execs, skillExecs...)
		// spec-2.3a D-4: register db_query when a connection can be selected
		// (without opening it — lazy; startup stays DB-I/O-free). No usable
		// connection → not registered + a differentiated debug log.
		execs = append(execs, DBQueryExecutors(cfg)...)
	}
	opts := llmapp.Options{
		ModelName:      cfg.LLM.ActiveModel,
		MaxHistory:     cfg.Session.MaxHistoryMessages,
		StripThink:     cfg.LLM.StripThink,
		ThinkingMode:   thinkingModeFromConfig(cfg.LLM.ThinkingMode),
		ThinkingBudget: cfg.LLM.ThinkingBudget,
		Registry:       diagnoseRegistryWith(execs...),
		SystemPrompt:   skillPrompt,
		// spec-1.21 D-6 user-config knobs reach the runtime here.
		// Per-turn LLM timeout reuses LLMConfig.RequestTimeout per spec
		// (NOT a duplicate Diagnose.* field) — the diagnose layer is
		// the orchestrator, the provider layer owns the request-level
		// timeout (cf. classifyTerminal's LLM.TIMEOUT mapping).
		MaxTurns:     cfg.Diagnose.MaxTurns,
		ToolTimeout:  cfg.Diagnose.ToolTimeout,
		TotalTimeout: cfg.Diagnose.TotalTimeout,
		ReqTimeout:   cfg.LLM.RequestTimeout,
		// spec-1.22: wire the dedup cache from config.
		DedupEnabled: cfg.Diagnose.DedupEnabled,
		DedupWindow:  cfg.Diagnose.DedupWindow,
	}
	if perr != nil {
		// Principle 3: explicit error, no demoapp fallback. Chrome (welcome
		// + status cwd/git) is NOT seeded on the error path (spec-1.25 D-2:
		// only the healthy interactive construction seeds it).
		return llmapp.New(fake.New().WithStartErr(perr), opts)
	}
	// spec-1.25 D-2/D-4: seed the welcome panel + rich status bar on the
	// healthy interactive path. cwd + git branch are read ONCE here (never on
	// the render frame; R-3). os.Getwd failure simply omits the cwd line.
	applyChromeOptions(&opts)
	return llmapp.New(provider, opts)
}

// applyChromeOptions populates the spec-1.25 chrome fields on opts for the
// interactive TUI path: enables the welcome seed and reads cwd + git branch
// once (stdlib only, no shell — see llmapp.GitBranch). Kept off the
// error-fallback / headless paths so the welcome appears only on a healthy
// interactive launch (D-2 canary).
func applyChromeOptions(opts *llmapp.Options) {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = ""
	}
	opts.Welcome = true
	opts.Version = version.String()
	opts.Cwd = llmapp.AbbrevCwd(cwd, os.Getenv("HOME"))
	opts.GitBranch = llmapp.GitBranch(cwd)
}

// stripThinkMigrationNoticeOnce is a per-process latch — the
// once-emitted contract is process-wide so re-entering newChatModel
// (tests, future hot-reload) does not spam the debug log.
//
//nolint:gochecknoglobals // spec-1.20.2 D-5 R-1: one-shot migration log; per-process state is the simplest correct shape.
var stripThinkMigrationNoticeOnce sync.Once

// stripThinkMigrationLogFn is a TEST SEAM sink for emitStripThinkMigrationNotice.
// Production wires it to logger.WarnForceFile (file-only, bypasses
// debug gate, never stderr — spec-1.20.2 D-4 contract). Tests override
// it to a capture closure so the assertion does not depend on the
// platform logger singleton's init/close ordering.
//
//nolint:gochecknoglobals // spec-1.20.2 D-5 R-1: function-pointer test seam; cheaper than logger.ResetForTesting export.
var stripThinkMigrationLogFn = func(msg string, kv ...any) {
	logger.WarnForceFile(msg, kv...)
}

// resetStripThinkMigrationNoticeForTest is a test seam; production
// code MUST NOT call this. spec-1.20.2 D-5 R-1: the once-latch is
// per-process so unit tests that exercise the emission path need a
// way to clear state between sub-cases.
func resetStripThinkMigrationNoticeForTest() {
	stripThinkMigrationNoticeOnce = sync.Once{}
}

// emitStripThinkMigrationNotice writes a one-time info record to the
// platform logger when the user gets the new spec-1.20.2 D-5 BREAKING
// default for LLMConfig.StripThink (false → true) without having opted
// out via yaml or OPENDBX_LLM_STRIP_THINK. Caller MUST have already
// installed the slog → logger bridge (see LaunchInteractiveTUI) so
// the record reaches the debug file and not the user's terminal.
//
// The notice fires when StripThink came from SourceDefault AND is true.
// An operator who explicitly set strip_think: true via yaml / env gets
// the same value but a non-default source — we skip the log there to
// avoid noise.
func emitStripThinkMigrationNotice(cfg *config.Config) {
	if cfg == nil || !cfg.LLM.StripThink {
		return
	}
	if cfg.Source("LLM.StripThink") != config.SourceDefault {
		return
	}
	stripThinkMigrationNoticeOnce.Do(func() {
		stripThinkMigrationLogFn(
			"spec-1.20.2 D-5: thinking content is hidden by default (llm.strip_think=true). "+
				"To restore previous behavior, set llm.strip_think=false in your config "+
				"or OPENDBX_LLM_STRIP_THINK=false.",
			"spec", "1.20.2", "deliverable", "D-5", "breaking", true,
		)
	})
}

// defaultDiagnoseRegistry returns the production ToolExecutor registry
// wired into every interact session (spec-1.21 D-3 / D-6 / T-9 user
// boundary). Tools are intentionally minimal and read-only at this stage:
//
//   - clock — current time (RFC3339 UTC); no input, no side effects.
//   - echo  — JSON-echoes input verbatim; no shell / SQL / filesystem.
//
// Real DB-touching skills (topsql / pg_settings_* / awr) land in
// spec-2.1 skill-registry along with dynamic per-config loading. Until
// then we keep the surface narrow so a misuse cannot leak an execution
// path. Construction errors from diagnose.NewRegistry are programmer
// errors (registry contract violations) and panic at startup so they
// surface immediately rather than at first message.
func defaultDiagnoseRegistry() *diagnose.Registry {
	return diagnoseRegistryWith()
}

// diagnoseRegistryWith builds the production registry (clock + echo)
// plus any extra executors — in practice the spec-2.3 SkillTool, whose
// construction errors were already handled (log + skip) by
// skillsForChat before reaching here. The panic below therefore stays
// the spec-1.21 programmer-error precedent (registry contract
// violations: dup/empty names), unreachable from the skills path.
func diagnoseRegistryWith(extra ...diagnose.ToolExecutor) *diagnose.Registry {
	execs := append([]diagnose.ToolExecutor{diagnose.ClockTool{}, diagnose.EchoTool{}}, extra...)
	reg, err := diagnose.NewRegistry(execs...)
	if err != nil {
		panic("bootstrap: diagnoseRegistryWith: " + err.Error())
	}
	return reg
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
