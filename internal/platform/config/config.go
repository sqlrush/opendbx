// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package config provides the typed Config tree + loader + validator +
// hot-reload interface for opendbx.
//
// Spec: opendbrb/specs/stage-0/spec-0.4-config-framework.md
//
// Per user R3 Q8 decision: this file ships **only** the 7 sub-structs that
// have concrete behavior in stage 0 / Stage 1 (Security/Output/LLM/Session/
// Sentinel/Trace/Scheduler — all carried from opendb). New sub-structs
// (Render / UI / Plugins / Memory / Hooks / MCP / CostTracker) are NOT
// pre-declared; they appear in their owning spec when needed (backward-
// compatible because old yaml without the new section gets zero-value).
//
// Per user R3 Q4 + Q6 decisions, every leaf field uses 5 struct tags:
//
//	`yaml:"..."`     — yaml.v3 marshal name (canonical config format)
//	`json:"..."`     — JSON Schema output + parity with CC settings.json
//	`env:"..."`      — explicit ENV variable name (no auto-derivation)
//	`validate:"..."` — required / min / max / oneof / regex / cross-field
//	`redact:"true"`  — secret field; masked in all dump paths
package config

import "time"

// Config is the top-level opendbx configuration tree.
//
// 7 sub-structs match opendb 老版 1:1 (per spec § 1.4 default = restore
// opendb behavior):
type Config struct {
	Security  SecurityConfig  `yaml:"security" json:"security"`
	Output    OutputConfig    `yaml:"output" json:"output"`
	LLM       LLMConfig       `yaml:"llm" json:"llm"`
	Session   SessionConfig   `yaml:"session" json:"session"`
	Sentinel  SentinelConfig  `yaml:"sentinel" json:"sentinel"`
	Trace     TraceConfig     `yaml:"trace" json:"trace"`
	Scheduler SchedulerConfig `yaml:"scheduler" json:"scheduler"`

	// Inline collections — schema in spec-1.19 (Connections) / spec-1.20 (Models).
	// Stage 0 contract is "field exists; full schema deferred".
	Connections []ConnectionConfig `yaml:"connections,omitempty" json:"connections,omitempty"`
	Models      []ModelConfig      `yaml:"models,omitempty" json:"models,omitempty"`

	// sources is set by Load(); not yaml-marshaled. Maps "Section.Field"
	// dotted path → SettingSource of last writer. Used by `admin config sources`.
	sources map[string]SettingSource `yaml:"-" json:"-"`
}

// SecurityConfig — security defaults (carried from opendb).
type SecurityConfig struct {
	DefaultLevel       uint8 `yaml:"default_level" json:"default_level" env:"OPENDBX_SECURITY_DEFAULT_LEVEL" validate:"min=0,max=10"`
	ConfirmOnDangerous bool  `yaml:"confirm_on_dangerous" json:"confirm_on_dangerous" env:"OPENDBX_SECURITY_CONFIRM_ON_DANGEROUS"`
}

// OutputConfig — terminal output formatting.
type OutputConfig struct {
	Format       string `yaml:"format" json:"format" env:"OPENDBX_OUTPUT_FORMAT" validate:"required,oneof=text json stream-json"`
	Color        string `yaml:"color" json:"color" env:"OPENDBX_OUTPUT_COLOR" validate:"required,oneof=auto always never"`
	WrapWidth    int    `yaml:"wrap_width" json:"wrap_width" env:"OPENDBX_OUTPUT_WRAP_WIDTH" validate:"min=0,max=500"`
	IncludeStats bool   `yaml:"include_stats" json:"include_stats" env:"OPENDBX_OUTPUT_INCLUDE_STATS"`
}

// LLMConfig — LLM provider configuration. Full provider list in spec-1.20.
type LLMConfig struct {
	ActiveModel    string        `yaml:"active_model" json:"active_model" env:"OPENDBX_LLM_ACTIVE_MODEL"`
	Tier           string        `yaml:"tier" json:"tier" env:"OPENDBX_LLM_TIER" validate:"required,oneof=tier-1 tier-2 tier-3 tier-4"`
	APIKey         string        `yaml:"api_key,omitempty" json:"api_key,omitempty" env:"OPENDBX_LLM_API_KEY" redact:"true"`
	BaseURL        string        `yaml:"base_url,omitempty" json:"base_url,omitempty" env:"OPENDBX_LLM_BASE_URL"`
	RequestTimeout time.Duration `yaml:"request_timeout" json:"request_timeout" env:"OPENDBX_LLM_REQUEST_TIMEOUT" validate:"min=1"`
	MaxRetries     int           `yaml:"max_retries" json:"max_retries" env:"OPENDBX_LLM_MAX_RETRIES" validate:"min=0,max=10"`
	StripThink     bool          `yaml:"strip_think" json:"strip_think" env:"OPENDBX_LLM_STRIP_THINK"`
	ThinkingMode   string        `yaml:"thinking_mode" json:"thinking_mode" env:"OPENDBX_LLM_THINKING_MODE" validate:"required,oneof=enabled disabled adaptive"`
}

// SessionConfig — session lifecycle + memory bounds.
type SessionConfig struct {
	StorageDir         string        `yaml:"storage_dir" json:"storage_dir" env:"OPENDBX_SESSION_STORAGE_DIR"`
	MaxHistoryMessages int           `yaml:"max_history_messages" json:"max_history_messages" env:"OPENDBX_SESSION_MAX_HISTORY_MESSAGES" validate:"min=1,max=1000"`
	IdleTimeout        time.Duration `yaml:"idle_timeout" json:"idle_timeout" env:"OPENDBX_SESSION_IDLE_TIMEOUT"`
	CompactionEnabled  bool          `yaml:"compaction_enabled" json:"compaction_enabled" env:"OPENDBX_SESSION_COMPACTION_ENABLED"`
	AuditEnabled       bool          `yaml:"audit_enabled" json:"audit_enabled" env:"OPENDBX_SESSION_AUDIT_ENABLED"`
}

// SentinelConfig — DB metric probe defaults. Full 48-metric thresholds in spec-3.6.
type SentinelConfig struct {
	Enabled           bool          `yaml:"enabled" json:"enabled" env:"OPENDBX_SENTINEL_ENABLED"`
	PollInterval      time.Duration `yaml:"poll_interval" json:"poll_interval" env:"OPENDBX_SENTINEL_POLL_INTERVAL" validate:"min=1"`
	WarmupSeconds     int           `yaml:"warmup_seconds" json:"warmup_seconds" env:"OPENDBX_SENTINEL_WARMUP_SECONDS" validate:"min=0,max=600"`
	NotifyChannels    []string      `yaml:"notify_channels,omitempty" json:"notify_channels,omitempty" env:"OPENDBX_SENTINEL_NOTIFY_CHANNELS"`
	HardCeilingFactor float64       `yaml:"hard_ceiling_factor" json:"hard_ceiling_factor" env:"OPENDBX_SENTINEL_HARD_CEILING_FACTOR" validate:"min=1,max=100"`
}

// TraceConfig — OpenTelemetry trace endpoint (spec-0.5 logger consumes).
type TraceConfig struct {
	Enabled    bool    `yaml:"enabled" json:"enabled" env:"OPENDBX_TRACE_ENABLED"`
	Endpoint   string  `yaml:"endpoint,omitempty" json:"endpoint,omitempty" env:"OPENDBX_TRACE_ENDPOINT"`
	SampleRate float64 `yaml:"sample_rate" json:"sample_rate" env:"OPENDBX_TRACE_SAMPLE_RATE" validate:"min=0,max=1"`
}

// SchedulerConfig — render/IO scheduler tuning (spec-1.4 firms up).
type SchedulerConfig struct {
	WorkerPoolSize  int           `yaml:"worker_pool_size" json:"worker_pool_size" env:"OPENDBX_SCHEDULER_WORKER_POOL_SIZE" validate:"min=1,max=128"`
	FrameBudget     time.Duration `yaml:"frame_budget" json:"frame_budget" env:"OPENDBX_SCHEDULER_FRAME_BUDGET" validate:"min=1"`
	MaxQueuedFrames int           `yaml:"max_queued_frames" json:"max_queued_frames" env:"OPENDBX_SCHEDULER_MAX_QUEUED_FRAMES" validate:"min=1,max=1024"`
}

// ConnectionConfig — DB connection schema. Stage 0 keeps minimal fields;
// spec-1.19-connection-config fills the rest.
type ConnectionConfig struct {
	Alias  string `yaml:"alias" json:"alias" validate:"required"`
	Driver string `yaml:"driver" json:"driver" validate:"oneof=postgres mysql oracle opengauss"`
	DSN    string `yaml:"dsn" json:"dsn" redact:"true"`
}

// ModelConfig — LLM model endpoint. Stage 0 minimal; spec-1.20 fills rest.
type ModelConfig struct {
	Name     string `yaml:"name" json:"name" validate:"required"`
	Provider string `yaml:"provider" json:"provider" validate:"oneof=anthropic openai-compat ollama"`
	BaseURL  string `yaml:"base_url" json:"base_url"`
	APIKey   string `yaml:"api_key,omitempty" json:"api_key,omitempty" redact:"true"`
}

// Watcher returns the hot-reload watcher. spec-0.4 ships NoopWatcher;
// spec-4.6 swaps in a real fsnotify implementation.
func (c *Config) Watcher() Watcher {
	return globalWatcher
}

// Source returns the SettingSource of the field at dotted path (e.g.
// "Security.DefaultLevel"). Returns SourceDefault if the field hasn't
// been overridden by any source above defaults.
func (c *Config) Source(field string) SettingSource {
	if c.sources == nil {
		return SourceDefault
	}
	if src, ok := c.sources[field]; ok {
		return src
	}
	return SourceDefault
}

// SetSource marks `field` (dotted path) as having been written by `src`.
// Called by Load() during the override chain walk.
func (c *Config) SetSource(field string, src SettingSource) {
	if c.sources == nil {
		c.sources = make(map[string]SettingSource)
	}
	c.sources[field] = src
}
