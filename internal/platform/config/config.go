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
	Diagnose  DiagnoseConfig  `yaml:"diagnose" json:"diagnose"`

	// DefaultConnection selects the active connection by alias when no
	// --connection-alias CLI flag is given (spec-1.19 D-1/D-6).
	DefaultConnection string `yaml:"default_connection,omitempty" json:"default_connection,omitempty"`

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
	LogLevel     string `yaml:"log_level" json:"log_level" env:"OPENDBX_OUTPUT_LOG_LEVEL" validate:"oneof=verbose debug info warn error"`
	LogPath      string `yaml:"log_path,omitempty" json:"log_path,omitempty" env:"OPENDBX_OUTPUT_LOG_PATH"`
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
	// ThinkingBudget is the extended-thinking token budget when ThinkingMode
	// is "enabled" (spec-1.20 R2.1 / T-10a HIGH-2 wiring). Anthropic requires
	// ≥1024 and < max_tokens; that bound is enforced at request time by
	// llm.ValidateRequest. Ignored when ThinkingMode != "enabled".
	ThinkingBudget int `yaml:"thinking_budget" json:"thinking_budget" env:"OPENDBX_LLM_THINKING_BUDGET" validate:"min=0"`
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

// DiagnoseConfig — multi-turn diagnose-loop tuning (spec-1.21 D-6).
//
// MaxTurns caps the number of LLM round-trips before DIAGNOSE.MAX_TURNS
// terminates the loop (default 16). ToolTimeout bounds a single
// ToolExecutor.Execute call (default 30s). TotalTimeout bounds the whole
// loop (default 10min). Per-turn LLM timeout reuses LLMConfig.RequestTimeout
// (not duplicated here — spec-1.21 D-6: "per-turn LLM 超时复用
// LLMConfig.RequestTimeout").
//
// Cross-field invariant (enforced in validation.go):
// ToolTimeout ≤ TotalTimeout — a tool deadline exceeding the loop budget
// is non-sensical (loop would terminate before the tool could complete).
type DiagnoseConfig struct {
	MaxTurns     int           `yaml:"max_turns" json:"max_turns" env:"OPENDBX_DIAGNOSE_MAX_TURNS" validate:"min=1,max=100"`
	ToolTimeout  time.Duration `yaml:"tool_timeout" json:"tool_timeout" env:"OPENDBX_DIAGNOSE_TOOL_TIMEOUT" validate:"min=1"`
	TotalTimeout time.Duration `yaml:"total_timeout" json:"total_timeout" env:"OPENDBX_DIAGNOSE_TOTAL_TIMEOUT" validate:"min=1"`
	// spec-1.22 tool dedup cache. DedupEnabled is the on/off switch (启停 is
	// the bool, NOT a magic DedupWindow==0 — codex HIGH-2). DedupWindow is the
	// turn-distance within which an identical call is served from cache; it is
	// intentionally NOT in the validateCrossField block (independent of
	// MaxTurns — a window > MaxTurns benignly degrades to whole-Run dedup, Q7).
	DedupEnabled bool `yaml:"dedup_enabled" json:"dedup_enabled" env:"OPENDBX_DIAGNOSE_DEDUP_ENABLED"`
	DedupWindow  int  `yaml:"dedup_window" json:"dedup_window" env:"OPENDBX_DIAGNOSE_DEDUP_WINDOW" validate:"min=1,max=100"`
}

// ConnectionConfig — DB connection schema (spec-1.19). Two mutually-exclusive
// modes (XOR, enforced by validateConnections):
//
//   - DSN mode: a full driver DSN string (any driver). Self-contained.
//   - fields mode: structured Host/Port/Database/User/Password/SSLMode
//     (postgres only — composed via the driver's db.DSNComposer capability).
//
// Host/Database/User/Port carry NO validate tag: the per-field validator
// cannot tell which mode a connection is in, so requiring them via tags would
// break DSN-mode configs. Their cross-field rules live in validateConnections.
// Password resolution prefers OPENDBX_DB_PASSWORD_<ALIAS> over the config
// field (see resolvePassword); the field itself is redacted in all dumps.
type ConnectionConfig struct {
	Alias    string `yaml:"alias" json:"alias" validate:"required"`
	Driver   string `yaml:"driver" json:"driver" validate:"oneof=postgres mysql oracle opengauss"`
	DSN      string `yaml:"dsn,omitempty" json:"dsn,omitempty" redact:"true"`
	Host     string `yaml:"host,omitempty" json:"host,omitempty"`
	Port     int    `yaml:"port,omitempty" json:"port,omitempty"`
	Database string `yaml:"database,omitempty" json:"database,omitempty"`
	User     string `yaml:"user,omitempty" json:"user,omitempty"`
	Password string `yaml:"password,omitempty" json:"password,omitempty" redact:"true"`
	SSLMode  string `yaml:"sslmode,omitempty" json:"sslmode,omitempty" validate:"oneof=disable allow prefer require verify-ca verify-full"`
}

// ModelConfig — LLM model endpoint. Stage 0 minimal; spec-1.20 fills rest.
type ModelConfig struct {
	Name     string `yaml:"name" json:"name" validate:"required"`
	Provider string `yaml:"provider" json:"provider" validate:"oneof=anthropic openai-compat ollama fake"`
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
