// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Config relay (spec-0.4 D-9). Routes cmd/opendbx → internal/platform/config
// through entrypoints so that the cmd → platform exception remains
// internal/platform/version only (per spec-0.3 hotfix rationale).
//
// This file mirrors profile_relay.go's pattern.

package entrypoints

import (
	"context"
	"io"
	"os"

	"github.com/sqlrush/opendbx/internal/platform/config"
)

// LoadConfig invokes config.Load with the given options. Returns the
// resolved Config or the first parse / validation error.
func LoadConfig(opts config.LoadOptions) (*config.Config, error) {
	return config.Load(opts)
}

// LoadConfigWithOptions is the canonical config-load entrypoint for cobra
// PersistentPreRunE: takes a fully-built LoadOptions (with FlagSettingsPath
// and FlagOverrides populated from cobra-parsed flags).
func LoadConfigWithOptions(opts config.LoadOptions) (*config.Config, error) {
	return config.Load(opts)
}

// FlagOverride mirrors config.FieldOverride; exposed via entrypoints so
// cmd/opendbx need not import internal/platform/config directly (preserves
// the cmd → platform/version single exception per spec-0.3 hotfix).
type FlagOverride struct {
	Path  string
	Value any
}

// CLILoadInputs is the cmd/opendbx → entrypoints → config.Load bridge.
type CLILoadInputs struct {
	CWD            string
	SettingsPath   string
	SettingSources string
	Overrides      []FlagOverride
}

// LoadConfigFromCLI builds config.LoadOptions from cmd-side inputs and
// invokes config.Load. cmd/opendbx/root.go imports only entrypoints.
func LoadConfigFromCLI(in CLILoadInputs) (*config.Config, error) {
	overrides := make([]config.FieldOverride, 0, len(in.Overrides))
	for _, o := range in.Overrides {
		overrides = append(overrides, config.FieldOverride{Path: o.Path, Value: o.Value})
	}
	return config.Load(config.LoadOptions{
		CWD:              in.CWD,
		FlagSettingsPath: in.SettingsPath,
		SettingSources:   in.SettingSources,
		FlagOverrides:    overrides,
	})
}

// LoadConfigDefault calls Load with cwd = os.Getwd() and no flag overrides.
// Used by admin config sources / dump-defaults / etc. when they need a
// fresh config view (e.g. for tests outside the cobra dispatch path).
func LoadConfigDefault() (*config.Config, error) {
	cwd, _ := os.Getwd()
	return config.Load(config.LoadOptions{CWD: cwd})
}

type configContextKey struct{}

// WithConfig returns a context with cfg attached. cmd/opendbx's
// PersistentPreRunE calls this; subcommand RunE call ConfigFromContext.
func WithConfig(parent context.Context, cfg *config.Config) context.Context {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithValue(parent, configContextKey{}, cfg)
}

// ConfigFromContext extracts a previously-attached *config.Config; returns
// nil if none was set (caller should fall back to LoadConfigDefault).
func ConfigFromContext(ctx context.Context) *config.Config {
	if ctx == nil {
		return nil
	}
	cfg, _ := ctx.Value(configContextKey{}).(*config.Config)
	return cfg
}

// HasConfigInContext is a typed predicate for cmd-side code that wants to
// avoid importing the config package merely to nil-check the context value.
func HasConfigInContext(ctx context.Context) bool {
	return ConfigFromContext(ctx) != nil
}

// DumpDefaults writes the redacted default Config as YAML to w.
func DumpDefaults(w io.Writer) error {
	return config.WriteDefaultsYAML(w)
}

// DumpSchema writes the JSON Schema describing Config to w.
func DumpSchema(w io.Writer) error {
	return config.WriteSchemaJSON(w)
}

// DumpEnvMap writes the ENV name → struct path mapping to w.
func DumpEnvMap(w io.Writer) error {
	return config.WriteEnvMap(w)
}

// DescribeSources writes per-field source provenance (or for a single
// dotted field if `field` non-empty) to w.
func DescribeSources(w io.Writer, cfg *config.Config, field string) error {
	return config.WriteSources(w, cfg, field)
}

// ValidateFile loads `path` (no other sources, just the single file) and
// runs Validate. Returns combined errors.
func ValidateFile(path string) error {
	return config.ValidateFile(path)
}
