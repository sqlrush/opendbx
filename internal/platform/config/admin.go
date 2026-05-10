// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Admin verb implementations (spec-0.4 D-8). Each function writes to a
// caller-provided io.Writer (so cmd/opendbx can redirect to cobra's
// Out/ErrOrStderr writers, and tests can capture into bytes.Buffer).

package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"sort"
	"strings"

	yaml "go.yaml.in/yaml/v3"
)

// WriteDefaultsYAML writes the redacted Default() Config as YAML.
func WriteDefaultsYAML(w io.Writer) error {
	defaults := Redact(Default())
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	if err := enc.Encode(defaults); err != nil {
		return fmt.Errorf("encode defaults: %w", err)
	}
	return enc.Close()
}

// WriteSchemaJSON writes the JSON Schema for Config to w.
func WriteSchemaJSON(w io.Writer) error {
	raw, err := SchemaJSON()
	if err != nil {
		return err
	}
	if _, err := w.Write(raw); err != nil {
		return err
	}
	if _, err := w.Write([]byte("\n")); err != nil {
		return err
	}
	return nil
}

// WriteEnvMap writes "OPENDBX_FOO  Section.Field" lines (sorted).
func WriteEnvMap(w io.Writer) error {
	m := EnvMap()
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if _, err := fmt.Fprintf(w, "%-50s %s\n", k, m[k]); err != nil {
			return err
		}
	}
	return nil
}

// WriteSources writes per-field source provenance.
//
// If field == "", lists all top-level Config sections + their source.
// Otherwise resolves the dotted field path and prints just that one row.
func WriteSources(w io.Writer, cfg *Config, field string) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	if field != "" {
		section := topSection(field)
		src := cfg.Source(section)
		_, err := fmt.Fprintf(w, "%-30s %s\n", field, src.String())
		return err
	}
	// All top-level sections.
	t := reflect.TypeOf(*cfg)
	for i := 0; i < t.NumField(); i++ {
		ft := t.Field(i)
		if !ft.IsExported() {
			continue
		}
		src := cfg.Source(ft.Name)
		if _, err := fmt.Fprintf(w, "%-30s %s\n", ft.Name, src.String()); err != nil {
			return err
		}
	}
	return nil
}

// ValidateFile loads ONE yaml file (ignoring other sources / ENV / CLI) and
// runs Validate against the result. Used by `admin config validate <file>`.
func ValidateFile(path string) error {
	if path == "" {
		return fmt.Errorf("validate: path is empty")
	}
	if !fileExists(path) {
		return fmt.Errorf("validate: file not found: %s", path)
	}
	raw, err := os.ReadFile(path) //nolint:gosec // user-supplied admin tool path
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	cfg := Default()
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true)
	var overlay Config
	if err := dec.Decode(&overlay); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	mergeOverlay(cfg, &overlay, SourceUserSettings)
	return Validate(cfg)
}

// commonRedacted produces the conventional redaction-aware print of the
// active config (used by admin sources / debug log path).
func commonRedacted(cfg *Config) string {
	var b strings.Builder
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	_ = enc.Encode(Redact(cfg))
	_ = enc.Close()
	return b.String()
}

// Ensure commonRedacted is referenced (avoid "unused" lint while keeping it
// available for spec-0.5 logger to import via package).
var _ = commonRedacted
