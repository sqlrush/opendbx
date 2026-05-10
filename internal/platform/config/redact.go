// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Redaction for secret fields (spec-0.4 D-6, R3 Q6* contract).
//
// Per user R3 Q6* decision: every dump / observability path that surfaces
// Config values must mask `redact:"true"`-tagged fields as "<REDACTED>".
// Affected paths:
//
//   - admin config dump-defaults / dump-schema / sources
//   - Validate error messages (validation.go::stringifyValue with redact=true)
//   - profile log / debug log (spec-0.5 wires these)
//   - trace span attributes (spec-0.5 wires these)
//
// Implementation strategy: produce a *Config copy with secrets masked, then
// hand it to yaml/json marshalers. The reflective copy avoids touching the
// real config while still letting yaml.v3 emit a full structural dump.

package config

import "reflect"

// RedactedSentinel is the literal placed in masked fields by Redact.
const RedactedSentinel = "<REDACTED>"

// Redact returns a deep copy of cfg with `redact:"true"` string fields
// replaced by RedactedSentinel. Non-string redact-tagged fields are zeroed
// (rare in practice — secrets are typically strings).
//
// The original cfg is NOT modified.
func Redact(cfg *Config) *Config {
	if cfg == nil {
		return nil
	}
	clone := *cfg
	// Shallow copy of slices — we then deep-clone & mask each element.
	if cfg.Connections != nil {
		clone.Connections = make([]ConnectionConfig, len(cfg.Connections))
		copy(clone.Connections, cfg.Connections)
	}
	if cfg.Models != nil {
		clone.Models = make([]ModelConfig, len(cfg.Models))
		copy(clone.Models, cfg.Models)
	}
	maskRedactFields(reflect.ValueOf(&clone).Elem())
	return &clone
}

// maskRedactFields walks v and replaces redact-tagged fields with sentinels.
func maskRedactFields(v reflect.Value) {
	if !v.IsValid() {
		return
	}
	switch v.Kind() {
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			fv := v.Field(i)
			ft := t.Field(i)
			if !ft.IsExported() {
				continue
			}
			if ft.Tag.Get("redact") == "true" {
				switch fv.Kind() {
				case reflect.String:
					if fv.CanSet() && fv.String() != "" {
						fv.SetString(RedactedSentinel)
					}
				default:
					if fv.CanSet() {
						fv.Set(reflect.Zero(fv.Type()))
					}
				}
				continue
			}
			maskRedactFields(fv)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			maskRedactFields(v.Index(i))
		}
	}
}
