// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// ENV variable application (spec-0.4 D-3, R3 Q4* explicit-tag mode).
//
// Per user R3 Q4* decision: NO field-name auto-derivation. Each Config
// field that participates in ENV must carry an `env:"OPENDBX_..."` struct
// tag; fields without the tag are silently skipped (cannot be set via ENV).
//
// This protects external deployment contracts (docker / k8s ENV files) from
// silent breakage when Go field names are renamed.

package config

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// applyENV walks Config fields, reads each `env:"OPENDBX_..."`-tagged field's
// ENV variable, and overwrites the Config value when present.
//
// Returns the first parse error (e.g. invalid duration) — spec § 3.1.
func applyENV(cfg *Config) error {
	return walkENVTags(reflect.ValueOf(cfg).Elem(), "", cfg)
}

// EnvMap returns the ENV name → "Section.Field" path mapping discovered via
// reflection. Used by `admin config dump-env-map` (D-8) and the corresponding
// golden test (D-3 contract).
func EnvMap() map[string]string {
	out := make(map[string]string)
	collectEnvTags(reflect.TypeOf(Config{}), "", out)
	return out
}

// walkENVTags traverses cfg by reflection, applying ENV overrides to fields
// with `env:"..."` tag. parentPath is the dotted Config path being walked.
func walkENVTags(v reflect.Value, parentPath string, cfg *Config) error {
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		fv := v.Field(i)
		ft := t.Field(i)
		if !ft.IsExported() {
			continue
		}
		path := joinPath(parentPath, ft.Name)

		envName := ft.Tag.Get("env")
		if envName == "" {
			// Recurse into nested structs (e.g. SecurityConfig fields).
			if fv.Kind() == reflect.Struct {
				if err := walkENVTags(fv, path, cfg); err != nil {
					return err
				}
			}
			continue
		}

		raw, ok := os.LookupEnv(envName)
		if !ok {
			continue
		}
		if err := assignFromString(fv, raw); err != nil {
			return fmt.Errorf("%w: ENV %s: %w", errENVParse, envName, err)
		}
		// Mark top-level section as ENV-sourced (parentPath empty path means
		// scalar-on-Config; tag-bearing fields live inside sub-structs).
		section := topSection(path)
		if section != "" {
			cfg.SetSource(section, SourceENV)
		}
		cfg.SetSource(path, SourceENV)
	}
	return nil
}

// collectEnvTags is the read-only twin of walkENVTags for EnvMap().
func collectEnvTags(t reflect.Type, parentPath string, out map[string]string) {
	for i := 0; i < t.NumField(); i++ {
		ft := t.Field(i)
		if !ft.IsExported() {
			continue
		}
		path := joinPath(parentPath, ft.Name)
		envName := ft.Tag.Get("env")
		if envName != "" {
			out[envName] = path
			continue
		}
		if ft.Type.Kind() == reflect.Struct {
			collectEnvTags(ft.Type, path, out)
		}
	}
}

// assignFromString parses `raw` according to fv's reflect.Kind / type and
// writes it. Supports string / int(8/16/32/64) / uint(8/16/32/64) / float64 /
// bool / time.Duration / []string (comma-separated).
func assignFromString(fv reflect.Value, raw string) error {
	if !fv.CanSet() {
		return fmt.Errorf("cannot set field")
	}
	// time.Duration check (it's int64 underlying).
	if fv.Type() == reflect.TypeOf(time.Duration(0)) {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return fmt.Errorf("parse duration %q: %w", raw, err)
		}
		fv.SetInt(int64(d))
		return nil
	}

	switch fv.Kind() {
	case reflect.String:
		fv.SetString(raw)
	case reflect.Bool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return fmt.Errorf("parse bool %q: %w", raw, err)
		}
		fv.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(raw, 10, fv.Type().Bits())
		if err != nil {
			return fmt.Errorf("parse int %q: %w", raw, err)
		}
		fv.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(raw, 10, fv.Type().Bits())
		if err != nil {
			return fmt.Errorf("parse uint %q: %w", raw, err)
		}
		fv.SetUint(n)
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(raw, fv.Type().Bits())
		if err != nil {
			return fmt.Errorf("parse float %q: %w", raw, err)
		}
		fv.SetFloat(f)
	case reflect.Slice:
		if fv.Type().Elem().Kind() != reflect.String {
			return fmt.Errorf("unsupported slice element kind %s", fv.Type().Elem().Kind())
		}
		parts := splitCSV(raw)
		out := reflect.MakeSlice(fv.Type(), len(parts), len(parts))
		for i, p := range parts {
			out.Index(i).SetString(p)
		}
		fv.Set(out)
	default:
		return fmt.Errorf("unsupported kind %s", fv.Kind())
	}
	return nil
}

func joinPath(parent, name string) string {
	if parent == "" {
		return name
	}
	return parent + "." + name
}

func topSection(path string) string {
	if i := strings.IndexByte(path, '.'); i >= 0 {
		return path[:i]
	}
	return path
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
	}
	return parts
}

// applyFlagOverrides applies CLI-flag-supplied overrides (highest priority).
// FieldOverride.Path is dotted Config path; Value is scalar.
func applyFlagOverrides(cfg *Config, overrides []FieldOverride) error {
	if len(overrides) == 0 {
		return nil
	}
	root := reflect.ValueOf(cfg).Elem()
	for _, ov := range overrides {
		fv, err := resolveDottedField(root, ov.Path)
		if err != nil {
			return fmt.Errorf("flag override %s: %w", ov.Path, err)
		}
		// For string-valued overrides, parse via assignFromString to handle
		// duration / bool conversions consistently.
		if s, ok := ov.Value.(string); ok {
			if err := assignFromString(fv, s); err != nil {
				return fmt.Errorf("flag override %s=%q: %w", ov.Path, s, err)
			}
		} else {
			rv := reflect.ValueOf(ov.Value)
			if rv.Type().AssignableTo(fv.Type()) {
				fv.Set(rv)
			} else if rv.Type().ConvertibleTo(fv.Type()) {
				fv.Set(rv.Convert(fv.Type()))
			} else {
				return fmt.Errorf("flag override %s: cannot assign %T to %s", ov.Path, ov.Value, fv.Type())
			}
		}
		section := topSection(ov.Path)
		if section != "" {
			cfg.SetSource(section, SourceCLIFlag)
		}
		cfg.SetSource(ov.Path, SourceCLIFlag)
	}
	return nil
}

func resolveDottedField(root reflect.Value, path string) (reflect.Value, error) {
	curr := root
	for _, seg := range strings.Split(path, ".") {
		if curr.Kind() != reflect.Struct {
			return reflect.Value{}, fmt.Errorf("not a struct at %q", seg)
		}
		next := curr.FieldByName(seg)
		if !next.IsValid() {
			return reflect.Value{}, fmt.Errorf("field %q not found", seg)
		}
		curr = next
	}
	return curr, nil
}
