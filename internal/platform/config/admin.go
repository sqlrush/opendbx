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
// If field == "", lists all leaf Config fields + their source.
// Otherwise resolves the dotted field path and prints just that one row.
func WriteSources(w io.Writer, cfg *Config, field string) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	if field != "" {
		if _, err := resolveDottedField(reflect.ValueOf(cfg).Elem(), field); err != nil {
			return fmt.Errorf("unknown config field %q: %w", field, err)
		}
		src := cfg.Source(field)
		_, err := fmt.Fprintf(w, "%-30s %s\n", field, src.String())
		return err
	}
	paths := configSourcePaths(reflect.TypeOf(Config{}), "")
	sort.Strings(paths)
	for _, path := range paths {
		if _, err := fmt.Fprintf(w, "%-30s %s\n", path, cfg.Source(path).String()); err != nil {
			return err
		}
	}
	return nil
}

// ValidateFile loads ONE yaml file (ignoring other sources / ENV / CLI) and
// runs Validate against the result. Used by `admin config validate <file>`.
//
// CFG-MED-01 fix: applies the same 1MB size limit + KnownFields strict mode
// as mergeFile.
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
	if len(raw) > 1<<20 {
		return fmt.Errorf("%s: file too large (>1MB)", path)
	}
	if depth := yamlMaxDepth(raw); depth >= 32 {
		return fmt.Errorf("%s: YAML nesting depth %d ≥ 32 (rejected per spec § 3.2 anti-bomb)", path, depth)
	}
	cfg := Default()
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true)
	if err := dec.Decode(cfg); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("parse %s: %w", path, err)
	}
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
