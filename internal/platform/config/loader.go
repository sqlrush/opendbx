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

	// SettingSources optionally restricts user/project/local settings layers.
	// Empty means all three, matching CC's default behavior.
	SettingSources string
}

// FieldOverride represents a single CLI-flag → Config-field write.
type FieldOverride struct {
	Path  string // dotted: "LLM.RequestTimeout"
	Value any    // scalar value (string/int/bool/duration)
}

// Load builds the final *Config by walking the override chain:
//
//	Default() → policy yaml → user yaml → project yaml → local yaml →
//	  ENV vars → flag-settings yaml/JSON string → CLI flag overrides
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
	selectedSources, err := parseSettingSources(opts.SettingSources)
	if err != nil {
		return nil, err
	}

	if err := mergeFile(cfg, paths.PolicyPath, SourcePolicySettings); err != nil {
		return nil, err
	}
	if selectedSources["user"] {
		if err := mergeFile(cfg, paths.UserPath, SourceUserSettings); err != nil {
			return nil, err
		}
	}
	if selectedSources["project"] {
		if err := mergeFile(cfg, paths.ProjectPath, SourceProjectSettings); err != nil {
			return nil, err
		}
	}
	if selectedSources["local"] {
		if err := mergeFile(cfg, paths.LocalPath, SourceLocalSettings); err != nil {
			return nil, err
		}
	}

	// Per spec § 1.1 D-2 override chain: ... → Local → ENV → --settings → CLI flag.
	// CFG-HIGH-02 fix: ENV must run BEFORE --settings (so --settings beats ENV).
	if err := applyENV(cfg); err != nil {
		return nil, err
	}
	if paths.FlagPath != "" {
		if err := mergeFlagSettings(cfg, paths.FlagPath); err != nil {
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

func parseSettingSources(raw string) (map[string]bool, error) {
	selected := map[string]bool{"user": true, "project": true, "local": true}
	if strings.TrimSpace(raw) == "" {
		return selected, nil
	}
	selected = map[string]bool{"user": false, "project": false, "local": false}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	})
	for _, part := range parts {
		if part == "" {
			continue
		}
		if _, ok := selected[part]; !ok {
			return nil, fmt.Errorf("invalid --setting-sources value %q (allowed: user, project, local)", part)
		}
		selected[part] = true
	}
	return selected, nil
}

func mergeFlagSettings(cfg *Config, arg string) error {
	trimmed := strings.TrimSpace(arg)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return mergeBytes(cfg, "<--settings>", []byte(trimmed), SourceFlagSettings)
	}
	if !fileExists(arg) {
		return fmt.Errorf("settings file not found: %s", arg)
	}
	return mergeFile(cfg, arg, SourceFlagSettings)
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
	return mergeBytes(cfg, path, raw, src)
}

func mergeBytes(cfg *Config, label string, raw []byte, src SettingSource) error {
	if len(raw) > 1<<20 {
		return fmt.Errorf("%s (%s source): file too large (>1MB)", label, src)
	}
	if depth := yamlMaxDepth(raw); depth >= 32 {
		return fmt.Errorf("%s (%s source): YAML nesting depth %d ≥ 32 (rejected per spec § 3.2 anti-bomb)", label, src, depth)
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
		return fmt.Errorf("parse %s (%s source): %w", label, src, err)
	}

	// Second pass: decode INTO cfg directly so untouched fields keep their
	// pre-existing values (Default or higher-priority overlay).
	dec2 := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec2.KnownFields(true)
	if err := dec2.Decode(cfg); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("parse %s (%s source): %w", label, src, err)
	}

	for _, key := range topLevelYAMLKeys(&rootNode) {
		section, ok := yamlKeyToSection(key)
		if !ok {
			continue // unknown keys are caught by KnownFields(true) above
		}
		cfg.SetSource(section, src)
	}
	for _, fieldPath := range yamlFieldPaths(&rootNode, reflect.TypeOf(Config{}), "") {
		cfg.SetSource(fieldPath, src)
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

func yamlFieldPaths(root *yaml.Node, t reflect.Type, parent string) []string {
	if root == nil || len(root.Content) == 0 {
		return nil
	}
	node := root
	if root.Kind == yaml.DocumentNode {
		node = root.Content[0]
	}
	return yamlFieldPathsFromMapping(node, t, parent)
}

func yamlFieldPathsFromMapping(node *yaml.Node, t reflect.Type, parent string) []string {
	if node == nil || node.Kind != yaml.MappingNode || t.Kind() != reflect.Struct {
		return nil
	}
	var paths []string
	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]
		if keyNode.Kind != yaml.ScalarNode {
			continue
		}
		ft, ok := fieldByYAMLName(t, keyNode.Value)
		if !ok {
			continue
		}
		path := joinPath(parent, ft.Name)
		if ft.Type.Kind() == reflect.Struct && valueNode.Kind == yaml.MappingNode {
			children := yamlFieldPathsFromMapping(valueNode, ft.Type, path)
			if len(children) > 0 {
				paths = append(paths, children...)
				continue
			}
		}
		paths = append(paths, path)
	}
	return paths
}

func fieldByYAMLName(t reflect.Type, name string) (reflect.StructField, bool) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		tag := f.Tag.Get("yaml")
		if comma := strings.IndexByte(tag, ','); comma >= 0 {
			tag = tag[:comma]
		}
		if tag == name {
			return f, true
		}
	}
	return reflect.StructField{}, false
}

func configSourcePaths(t reflect.Type, parent string) []string {
	if t.Kind() != reflect.Struct {
		return nil
	}
	var paths []string
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() || f.Tag.Get("yaml") == "-" {
			continue
		}
		path := joinPath(parent, f.Name)
		if f.Type.Kind() == reflect.Struct {
			children := configSourcePaths(f.Type, path)
			if len(children) > 0 {
				paths = append(paths, children...)
				continue
			}
		}
		paths = append(paths, path)
	}
	return paths
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
	for _, path := range configSourcePaths(reflect.TypeOf(Config{}), "") {
		cfg.SetSource(path, src)
	}
}
