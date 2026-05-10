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
		},
		LLM: LLMConfig{
			ActiveModel:    "",
			Tier:           "tier-1",
			APIKey:         "",
			BaseURL:        "",
			RequestTimeout: 30 * time.Second,
			MaxRetries:     3,
			StripThink:     false,
			ThinkingMode:   "adaptive",
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
		Connections: nil, // user must add via `opendbx db add` or yaml
		Models:      nil, // user must add via yaml or `opendbx auth login`
	}
}
