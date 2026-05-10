// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Multi-source config loader (spec-0.4 D-2). Override chain per SettingSource
// priority order. Fails-fast on parse / validation errors per spec § 3.1.

package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	yaml "go.yaml.in/yaml/v3"
)

// LoadOptions controls Load behavior.
type LoadOptions struct {
	// CWD is the project working directory (used to resolve project / local
	// .opendbx/ paths). Empty = skip project/local layers.
	CWD string

	// FlagSettingsPath, if non-empty, is the path supplied by `--settings`
	// flag. Replaces the SourceFlagSettings layer.
	FlagSettingsPath string

	// FlagOverrides is the slice of explicit (Config-field, value) overrides
	// from cmd/opendbx CLI flags (highest priority — SourceCLIFlag).
	// Caller (cmd/opendbx) builds this from Options struct.
	FlagOverrides []FieldOverride
}

// FieldOverride represents a single CLI-flag → Config-field write.
type FieldOverride struct {
	Path  string // dotted: "LLM.RequestTimeout"
	Value any    // scalar value (string/int/bool/duration)
}

// Load builds the final *Config by walking the override chain:
//
//	Default() → policy yaml → user yaml → project yaml → local yaml →
//	  flag-settings yaml → ENV vars → CLI flag overrides
//
// Returns the final Config + first parse/validation error (if any).
func Load(opts LoadOptions) (*Config, error) {
	cfg := Default()
	markAllAsSource(cfg, SourceDefault)

	paths, err := DefaultSourcePaths(opts.CWD)
	if err != nil {
		return nil, fmt.Errorf("resolve config paths: %w", err)
	}
	if opts.FlagSettingsPath != "" {
		paths.FlagPath = opts.FlagSettingsPath
	}

	if err := mergeFile(cfg, paths.PolicyPath, SourcePolicySettings); err != nil {
		return nil, err
	}
	if err := mergeFile(cfg, paths.UserPath, SourceUserSettings); err != nil {
		return nil, err
	}
	if err := mergeFile(cfg, paths.ProjectPath, SourceProjectSettings); err != nil {
		return nil, err
	}
	if err := mergeFile(cfg, paths.LocalPath, SourceLocalSettings); err != nil {
		return nil, err
	}
	if paths.FlagPath != "" {
		// --settings <path>: file MUST exist (per spec § 3.1 fail-fast).
		if !fileExists(paths.FlagPath) {
			return nil, fmt.Errorf("settings file not found: %s", paths.FlagPath)
		}
		if err := mergeFile(cfg, paths.FlagPath, SourceFlagSettings); err != nil {
			return nil, err
		}
	}

	if err := applyENV(cfg); err != nil {
		return nil, err
	}
	if err := applyFlagOverrides(cfg, opts.FlagOverrides); err != nil {
		return nil, err
	}

	if err := Validate(cfg); err != nil {
		return nil, fmt.Errorf("config validation failed:\n%w", err)
	}
	return cfg, nil
}

// mergeFile reads `path` (if it exists) and merges it into cfg, marking
// each top-level section that has any non-zero field with `src`.
//
// File-not-found is a no-op (silent skip — spec § 3.1 only fails on parse
// errors). YAML parse errors propagate immediately.
func mergeFile(cfg *Config, path string, src SettingSource) error {
	if path == "" || !fileExists(path) {
		return nil
	}
	raw, err := os.ReadFile(path) //nolint:gosec // operator-supplied config path
	if err != nil {
		return fmt.Errorf("read %s (%s source): %w", path, src, err)
	}
	if len(raw) > 1<<20 {
		return fmt.Errorf("%s (%s source): file too large (>1MB)", path, src)
	}
	// yaml.v3 strict mode: rejects unknown top-level fields per Q8 翻盘 ★B
	// philosophy (strict-unknown protects users from typos / forward-incompat
	// configs).
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true)

	var overlay Config
	if err := dec.Decode(&overlay); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return fmt.Errorf("parse %s (%s source): %w", path, src, err)
	}
	mergeOverlay(cfg, &overlay, src)
	return nil
}

// mergeOverlay assigns non-zero sub-structs from `overlay` to `cfg` and
// marks the source. Inline collections (Connections / Models) are
// REPLACED by overlay (not merged element-wise) when overlay's slice is
// non-nil.
func mergeOverlay(cfg, overlay *Config, src SettingSource) {
	// Each sub-struct: assign overlay's whole struct only if it differs from
	// the zero-value (i.e. yaml decoder wrote something). yaml.v3 leaves
	// untouched fields at zero-value.
	if (overlay.Security != SecurityConfig{}) {
		cfg.Security = overlay.Security
		cfg.SetSource("Security", src)
	}
	if (overlay.Output != OutputConfig{}) {
		cfg.Output = overlay.Output
		cfg.SetSource("Output", src)
	}
	if (overlay.LLM != LLMConfig{}) {
		cfg.LLM = overlay.LLM
		cfg.SetSource("LLM", src)
	}
	if (overlay.Session != SessionConfig{}) {
		cfg.Session = overlay.Session
		cfg.SetSource("Session", src)
	}
	// Sentinel / Trace contain non-comparable slices in some configs; compare
	// via JSON-equivalent zero-check.
	if !sentinelIsZero(overlay.Sentinel) {
		cfg.Sentinel = overlay.Sentinel
		cfg.SetSource("Sentinel", src)
	}
	if (overlay.Trace != TraceConfig{}) {
		cfg.Trace = overlay.Trace
		cfg.SetSource("Trace", src)
	}
	if (overlay.Scheduler != SchedulerConfig{}) {
		cfg.Scheduler = overlay.Scheduler
		cfg.SetSource("Scheduler", src)
	}
	if overlay.Connections != nil {
		cfg.Connections = overlay.Connections
		cfg.SetSource("Connections", src)
	}
	if overlay.Models != nil {
		cfg.Models = overlay.Models
		cfg.SetSource("Models", src)
	}
}

func sentinelIsZero(s SentinelConfig) bool {
	return !s.Enabled && s.PollInterval == 0 && s.WarmupSeconds == 0 &&
		s.NotifyChannels == nil && s.HardCeilingFactor == 0
}

// markAllAsSource marks every top-level field as having `src` provenance.
// Used at Load() entry to seed sources map for Default values.
func markAllAsSource(cfg *Config, src SettingSource) {
	for _, name := range []string{"Security", "Output", "LLM", "Session", "Sentinel", "Trace", "Scheduler", "Connections", "Models"} {
		cfg.SetSource(name, src)
	}
}
