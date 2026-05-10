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
	"io"
	"os"

	"github.com/sqlrush/opendbx/internal/platform/config"
)

// LoadConfig invokes config.Load with the given options. Returns the
// resolved Config or the first parse / validation error.
func LoadConfig(opts config.LoadOptions) (*config.Config, error) {
	return config.Load(opts)
}

// LoadConfigDefault calls Load with cwd = os.Getwd() and no flag overrides.
// Used by `cmd/opendbx/main.go` before cobra.Execute.
func LoadConfigDefault() (*config.Config, error) {
	cwd, _ := os.Getwd()
	return config.Load(config.LoadOptions{CWD: cwd})
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
