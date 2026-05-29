// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Defaults — Default() *Config returns the complete default configuration
// (spec-0.4 D-6).
//
// Invariant: Default() must NOT depend on any external resource (filesystem,
// ENV, network, time-of-day). Calling Default() twice in the same process
// must return value-equivalent (deep-equal) configs. The integration test
// E2E #3 enforces dump → Load round-trip identity.
//
// Secret fields (LLM.APIKey / Connection.DSN / Model.APIKey) default to "".
// Per user R3 Q6 decision, Default() never embeds placeholder secrets like
// "<change-me>"; loaders that need to detect "user-supplied vs default" can
// compare against zero-value.

package config

import "time"

// Default returns a fresh, fully-populated Config with sensible defaults.
//
// Numeric / duration / bool defaults are chosen to be safe in production
// (no surprise high-resource consumption, no permissive security mode).
func Default() *Config {
	return &Config{
		Security: SecurityConfig{
			DefaultLevel:       0, // 0 = no extra restrictions; spec-4.X security-baseline tightens
			ConfirmOnDangerous: true,
		},
		Output: OutputConfig{
			Format:       "text",
			Color:        "auto",
			WrapWidth:    0, // 0 = use terminal width
			IncludeStats: false,
			LogLevel:     "debug",
			LogPath:      "",
		},
		LLM: LLMConfig{
			ActiveModel:    "",
			Tier:           "tier-1",
			APIKey:         "",
			BaseURL:        "",
			RequestTimeout: 30 * time.Second,
			MaxRetries:     3,
			// spec-1.20.2 D-5 BREAKING: default flips false → true so
			// reasoning/thinking tokens do not pollute the main answer
			// out of the box (matches user expectation "show me the
			// answer, hide the chain-of-thought"). Operators who want
			// to see thinking content can opt in via strip_think:
			// false in their config or OPENDBX_LLM_STRIP_THINK=false.
			StripThink: true,
			// T-10a HIGH-1: default to "disabled" so a freshly-configured
			// install reaches the Anthropic provider for a real conversation.
			// "adaptive" is spec-3.11 (factory → LLM.NOT_IMPLEMENTED), so it
			// must NOT be the out-of-box default (the deliverable is a live
			// chat once an API key + model are set).
			ThinkingMode:   "disabled",
			ThinkingBudget: 2048, // used only when ThinkingMode == "enabled"
		},
		Session: SessionConfig{
			StorageDir:         "", // empty = use ~/.opendbx/sessions/
			MaxHistoryMessages: 20,
			IdleTimeout:        10 * time.Minute,
			CompactionEnabled:  true,
			AuditEnabled:       true,
		},
		Sentinel: SentinelConfig{
			Enabled:           false, // off until spec-1+ sentinel skeleton lands
			PollInterval:      10 * time.Second,
			WarmupSeconds:     30,
			NotifyChannels:    nil,
			HardCeilingFactor: 3.0,
		},
		Trace: TraceConfig{
			Enabled:    false,
			Endpoint:   "",
			SampleRate: 0.0,
		},
		Scheduler: SchedulerConfig{
			WorkerPoolSize:  4,
			FrameBudget:     16 * time.Millisecond, // 60fps target
			MaxQueuedFrames: 64,
		},
		Diagnose: DiagnoseConfig{
			// spec-1.21 D-6 defaults: 16 round-trips ceiling, 30s per
			// tool, 10min total. Per-turn LLM timeout is not duplicated
			// here — bootstrap reuses LLMConfig.RequestTimeout.
			MaxTurns:     16,
			ToolTimeout:  30 * time.Second,
			TotalTimeout: 10 * time.Minute,
		},
		Connections: nil, // user must add via `opendbx db add` or yaml
		Models:      nil, // user must add via yaml or `opendbx auth login`
	}
}
