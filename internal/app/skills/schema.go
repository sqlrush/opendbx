// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File schema.go — Schema mirrors the CC SKILL.md frontmatter byte-faithfully
// (spec-2.1 D-1; survey § 3.1/3.2 @ CC SHA 3da94d5).
//
// Q6=B: NO opendbx_ explicit fields. Extensions live in Extra until a real
// consumer (spec-2.3/2.4) promotes them to typed fields (§3.7 append-only).

package skills

import (
	"reflect"
	"strings"
)

// Schema is the parsed SKILL.md frontmatter. The four CC fields carry yaml
// tags and are decoded directly; Extra (yaml:"-") is populated separately by
// Parse from every non-CC-known top-level key (forward-compat, never dropped).
type Schema struct {
	Name         string         `yaml:"name"`          // required, frontmatter-authoritative
	Description  string         `yaml:"description"`   // required; block-scalar trailing \n preserved
	AllowedTools string         `yaml:"allowed-tools"` // optional CSV; "" = inherit parent scope (CC)
	Model        string         `yaml:"model"`         // optional; string superset, validated at invocation (2.3)
	Extra        map[string]any `yaml:"-"`             // all non-CC-known keys (incl user opendbx_*)
}

// AllowedToolsList splits the CSV, trims each element, and drops empties.
// Pure (no side effects). An absent / blank AllowedTools returns nil, which
// callers (spec-2.3) treat as "inherit parent scope" — distinct from a
// present-but-empty list.
func (s Schema) AllowedToolsList() []string {
	if strings.TrimSpace(s.AllowedTools) == "" {
		return nil
	}
	parts := strings.Split(s.AllowedTools, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// knownKeys is the set of frontmatter keys that map to an explicit Schema
// field, computed once at init (review MED: was recomputed per Parse).
var knownKeys = knownSchemaKeys()

// knownSchemaKeys returns the set of yaml frontmatter keys that map to an
// explicit Schema field. Derived via reflection over the yaml struct tags so
// it cannot drift from the struct definition (review: drift-proof known-key
// set). The yaml:"-" Extra field is excluded.
func knownSchemaKeys() map[string]bool {
	t := reflect.TypeOf(Schema{})
	keys := make(map[string]bool, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("yaml")
		if tag == "" || tag == "-" {
			continue
		}
		// Strip any tag options (e.g. ",omitempty").
		if comma := strings.IndexByte(tag, ','); comma >= 0 {
			tag = tag[:comma]
		}
		keys[tag] = true
	}
	return keys
}
