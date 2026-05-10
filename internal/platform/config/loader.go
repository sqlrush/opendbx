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
	"reflect"
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

	// Per spec § 1.1 D-2 override chain: ... → Local → ENV → --settings → CLI flag.
	// CFG-HIGH-02 fix: ENV must run BEFORE --settings (so --settings beats ENV).
	if err := applyENV(cfg); err != nil {
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
	if err := applyFlagOverrides(cfg, opts.FlagOverrides); err != nil {
		return nil, err
	}

	if err := Validate(cfg); err != nil {
		return nil, fmt.Errorf("config validation failed:\n%w", err)
	}
	return cfg, nil
}

// mergeFile reads `path` (if it exists) and merges it into cfg field-by-field
// (preserves existing default values for fields not present in the YAML).
//
// File-not-found is a no-op (silent skip — spec § 3.1 only fails on parse
// errors). YAML parse errors propagate immediately.
//
// Per CFG-HIGH-01 fix: decodes directly into cfg (yaml.v3 leaves
// untouched fields alone), so `output: {format: json}` no longer wipes
// `Output.Color="auto"` etc. Section provenance is recorded by walking the
// yaml.Node tree to find which top-level sections appear in the file.
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
	if depth := yamlMaxDepth(raw); depth >= 32 {
		return fmt.Errorf("%s (%s source): YAML nesting depth %d ≥ 32 (rejected per spec § 3.2 anti-bomb)", path, src, depth)
	}

	// First pass: parse into yaml.Node to discover which top-level keys
	// the file contains. This drives section-level provenance tracking
	// (cfg.SetSource(<top-level key name>, src)).
	var rootNode yaml.Node
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true)
	if err := dec.Decode(&rootNode); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return fmt.Errorf("parse %s (%s source): %w", path, src, err)
	}

	// Second pass: decode INTO cfg directly so untouched fields keep their
	// pre-existing values (Default or higher-priority overlay).
	dec2 := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec2.KnownFields(true)
	if err := dec2.Decode(cfg); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("parse %s (%s source): %w", path, src, err)
	}

	for _, key := range topLevelYAMLKeys(&rootNode) {
		section, ok := yamlKeyToSection(key)
		if !ok {
			continue // unknown keys are caught by KnownFields(true) above
		}
		cfg.SetSource(section, src)
	}
	return nil
}

// topLevelYAMLKeys returns the top-level scalar keys of the YAML document.
func topLevelYAMLKeys(root *yaml.Node) []string {
	if root == nil || len(root.Content) == 0 {
		return nil
	}
	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode {
		return nil
	}
	out := make([]string, 0, len(doc.Content)/2)
	for i := 0; i < len(doc.Content); i += 2 {
		if doc.Content[i].Kind == yaml.ScalarNode {
			out = append(out, doc.Content[i].Value)
		}
	}
	return out
}

// yamlKeyToSection maps a YAML top-level key (lowercase) to its Config
// struct field name (PascalCase). E.g. "security" → "Security".
func yamlKeyToSection(yamlKey string) (string, bool) {
	t := reflect.TypeOf(Config{})
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("yaml")
		// strip ",omitempty" and similar trailers
		if comma := strings.IndexByte(tag, ','); comma >= 0 {
			tag = tag[:comma]
		}
		if tag == yamlKey {
			return f.Name, true
		}
	}
	return "", false
}

// yamlMaxDepth scans `raw` and returns the maximum indentation-step depth
// observed. Used by mergeFile to enforce the spec § 3.2 anti-bomb depth ≥ 32
// rejection. Heuristic: count the maximum number of leading-space increments
// in the file. Cheap and good enough for the spec requirement.
func yamlMaxDepth(raw []byte) int {
	max := 0
	for _, line := range strings.Split(string(raw), "\n") {
		// Skip blank / pure-comment lines.
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		spaces := 0
		for _, ch := range line {
			if ch == ' ' {
				spaces++
			} else {
				break
			}
		}
		// 2-space indent convention; depth = spaces/2.
		depth := spaces / 2
		if depth > max {
			max = depth
		}
	}
	return max
}

// markAllAsSource marks every top-level field as having `src` provenance.
// Used at Load() entry to seed sources map for Default values.
func markAllAsSource(cfg *Config, src SettingSource) {
	for _, name := range []string{"Security", "Output", "LLM", "Session", "Sentinel", "Trace", "Scheduler", "Connections", "Models"} {
		cfg.SetSource(name, src)
	}
}
