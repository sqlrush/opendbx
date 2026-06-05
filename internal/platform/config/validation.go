// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Self-contained reflect-based validator (spec-0.4 D-4, R3 Q3 ★A).
//
// Supports `validate:"..."` struct tags with the rules:
//   - required           (string non-empty / numeric non-zero)
//   - min=N / max=N      (numeric / string-length / slice-len)
//   - oneof=a b c        (string ∈ allowed set)
//
// Returns a ValidationErrors slice with field path + rule + actual + expected;
// the report is human-friendly + machine-parseable (JSON-marshalable).

package config

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// ValidationError records one failed validation rule.
type ValidationError struct {
	Path     string `json:"path"`     // dotted Config path
	Rule     string `json:"rule"`     // "required" / "min" / "max" / "oneof"
	Expected string `json:"expected"` // rule expression
	Actual   string `json:"actual"`   // observed value (may be redacted)
	Source   string `json:"source"`   // which SettingSource set this value
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s violation (expected %s, got %s, source=%s)",
		e.Path, e.Rule, e.Expected, e.Actual, e.Source)
}

// ValidationErrors aggregates multiple errors; satisfies error interface.
type ValidationErrors []ValidationError

func (es ValidationErrors) Error() string {
	if len(es) == 0 {
		return "no validation errors"
	}
	var b strings.Builder
	for i, e := range es {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("  - ")
		b.WriteString(e.Error())
	}
	return b.String()
}

// Validate walks cfg, applying all `validate:"..."` rules + the small set
// of cross-field invariants the per-field tag grammar (required / min /
// max / oneof) cannot express. Returns nil when all pass;
// ValidationErrors otherwise (which is also error-typed).
func Validate(cfg *Config) error {
	var errs ValidationErrors
	walkValidate(reflect.ValueOf(cfg).Elem(), "", cfg, &errs)
	validateCrossField(cfg, &errs)
	if len(errs) > 0 {
		return wrapValidationError(errs)
	}
	return nil
}

// validateCrossField checks the invariants that span multiple fields.
// Keep the list small and explicit — adding a generic cross-field tag
// grammar to walkValidate would balloon the validator surface.
func validateCrossField(cfg *Config, errs *ValidationErrors) {
	if cfg == nil {
		return
	}
	// spec-1.21 D-6: a tool deadline exceeding the loop budget is
	// non-sensical (the loop would terminate before any single tool
	// could complete). Both fields are validated for min=1 individually
	// upstream; here we only check the inequality.
	if cfg.Diagnose.ToolTimeout > 0 && cfg.Diagnose.TotalTimeout > 0 &&
		cfg.Diagnose.ToolTimeout > cfg.Diagnose.TotalTimeout {
		src := cfg.Source("Diagnose.ToolTimeout")
		*errs = append(*errs, ValidationError{
			Path:     "Diagnose.ToolTimeout",
			Rule:     "ltefield",
			Expected: "Diagnose.ToolTimeout ≤ Diagnose.TotalTimeout (spec-1.21 D-6)",
			Actual:   cfg.Diagnose.ToolTimeout.String() + " > " + cfg.Diagnose.TotalTimeout.String(),
			Source:   src.String(),
		})
	}

	validateConnections(cfg, errs)
	validatePlugins(cfg, errs) // spec-2.2 D-6: skill search path traversal + count guard
}

// validateConnections enforces the spec-1.19 ConnectionConfig cross-field
// invariants the per-field tag grammar cannot express: DSN-xor-fields,
// fields-mode required trio, port range, and alias / env-key uniqueness.
// Never echoes a password/DSN value (those fields are redacted; we report
// structural facts only).
func validateConnections(cfg *Config, errs *ValidationErrors) {
	src := cfg.Source("Connections").String()
	seenAlias := map[string]int{}
	seenEnvKey := map[string]string{} // envKey → first alias

	for i, conn := range cfg.Connections {
		base := fmt.Sprintf("Connections[%d]", i)

		// XOR: exactly one of DSN-mode / fields-mode.
		dsnSet := conn.DSN != ""
		fieldsSet := conn.Host != "" || conn.Database != "" || conn.User != ""
		if dsnSet == fieldsSet { // both set (conflict) or neither (empty)
			actual := "neither dsn nor host/database/user set"
			if dsnSet {
				actual = "both dsn and structured fields set"
			}
			*errs = append(*errs, ValidationError{
				Path: base, Rule: "dsn-xor-fields",
				Expected: "exactly one of {dsn} or {host/database/user}",
				Actual:   actual, Source: src,
			})
		} else if fieldsSet { // fields mode: require the full trio
			for _, m := range []struct{ name, val string }{
				{"host", conn.Host}, {"database", conn.Database}, {"user", conn.User},
			} {
				if m.val == "" {
					*errs = append(*errs, ValidationError{
						Path: base + "." + m.name, Rule: "required-in-fields-mode",
						Expected: "non-empty in fields mode", Actual: "empty", Source: src,
					})
				}
			}
		}

		// Port range (0 means unset → driver default; only flag out-of-range).
		if conn.Port != 0 && (conn.Port < 1 || conn.Port > 65535) {
			*errs = append(*errs, ValidationError{
				Path: base + ".port", Rule: "range",
				Expected: "1..65535", Actual: strconv.Itoa(conn.Port), Source: src,
			})
		}

		// Alias uniqueness.
		if prev, dup := seenAlias[conn.Alias]; dup {
			*errs = append(*errs, ValidationError{
				Path: base + ".alias", Rule: "unique",
				Expected: "alias unique across connections",
				Actual:   fmt.Sprintf("%q also at Connections[%d]", conn.Alias, prev), Source: src,
			})
		} else {
			seenAlias[conn.Alias] = i
		}

		// Normalised env-key uniqueness (prod-db and prod_db collide).
		ek := envKey(conn.Alias)
		if other, dup := seenEnvKey[ek]; dup && other != conn.Alias {
			*errs = append(*errs, ValidationError{
				Path: base + ".alias", Rule: "envkey-unique",
				Expected: "aliases must map to distinct " + passwordEnvPrefix + "<KEY>",
				Actual:   fmt.Sprintf("%q and %q both → %s%s", conn.Alias, other, passwordEnvPrefix, ek),
				Source:   src,
			})
		} else if !dup {
			seenEnvKey[ek] = conn.Alias
		}
	}
}

func walkValidate(v reflect.Value, parentPath string, cfg *Config, errs *ValidationErrors) {
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		fv := v.Field(i)
		ft := t.Field(i)
		if !ft.IsExported() {
			continue
		}
		path := joinPath(parentPath, ft.Name)

		rules := ft.Tag.Get("validate")
		if rules != "" {
			srcLabel := SourceDefault.String()
			if cfg != nil {
				src := cfg.Source(path)
				if src == SourceDefault {
					src = cfg.Source(topSection(path))
				}
				srcLabel = src.String()
			}
			redacted := ft.Tag.Get("redact") == "true"
			actual := stringifyValue(fv, redacted)
			for _, rule := range strings.Split(rules, ",") {
				rule = strings.TrimSpace(rule)
				if rule == "" {
					continue
				}
				if err := applyRule(rule, fv); err != nil {
					*errs = append(*errs, ValidationError{
						Path:     path,
						Rule:     ruleName(rule),
						Expected: rule,
						Actual:   actual,
						Source:   srcLabel,
					})
				}
			}
		}

		// Recurse into nested struct fields.
		if fv.Kind() == reflect.Struct {
			walkValidate(fv, path, cfg, errs)
		}
		if fv.Kind() == reflect.Slice && fv.Type().Elem().Kind() == reflect.Struct {
			for j := 0; j < fv.Len(); j++ {
				elem := fv.Index(j)
				walkValidate(elem, fmt.Sprintf("%s[%d]", path, j), cfg, errs)
			}
		}
	}
}

// applyRule evaluates one rule expression against fv, returning nil on pass.
func applyRule(rule string, fv reflect.Value) error {
	switch {
	case rule == "required":
		return checkRequired(fv)
	case strings.HasPrefix(rule, "min="):
		return checkBoundary(fv, rule[len("min="):], boundaryMin)
	case strings.HasPrefix(rule, "max="):
		return checkBoundary(fv, rule[len("max="):], boundaryMax)
	case strings.HasPrefix(rule, "oneof="):
		allowed := strings.Fields(rule[len("oneof="):])
		return checkOneOf(fv, allowed)
	default:
		return fmt.Errorf("unknown rule %q", rule)
	}
}

func ruleName(rule string) string {
	if i := strings.IndexByte(rule, '='); i >= 0 {
		return rule[:i]
	}
	return rule
}

func checkRequired(fv reflect.Value) error {
	if fv.IsZero() {
		return fmt.Errorf("required")
	}
	return nil
}

type boundaryKind int

const (
	boundaryMin boundaryKind = iota
	boundaryMax
)

func checkBoundary(fv reflect.Value, valStr string, kind boundaryKind) error {
	bound, err := strconv.ParseFloat(valStr, 64)
	if err != nil {
		return fmt.Errorf("boundary parse: %w", err)
	}
	var actual float64
	switch fv.Kind() {
	case reflect.String:
		actual = float64(len(fv.String()))
	case reflect.Slice, reflect.Array, reflect.Map:
		actual = float64(fv.Len())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		// time.Duration is also Int64; treat as plain int for comparison
		// purposes (callers should express duration bounds via min/max in
		// ns or via dedicated rule — stage-0 keeps it simple).
		actual = float64(fv.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		actual = float64(fv.Uint())
	case reflect.Float32, reflect.Float64:
		actual = fv.Float()
	default:
		return fmt.Errorf("boundary unsupported kind %s", fv.Kind())
	}
	if kind == boundaryMin && actual < bound {
		return fmt.Errorf("min")
	}
	if kind == boundaryMax && actual > bound {
		return fmt.Errorf("max")
	}
	return nil
}

func checkOneOf(fv reflect.Value, allowed []string) error {
	got := stringifyValue(fv, false)
	// Empty values pass oneof (use `required` to forbid empty).
	if got == "" {
		return nil
	}
	for _, a := range allowed {
		if got == a {
			return nil
		}
	}
	return fmt.Errorf("oneof")
}

// stringifyValue converts fv to a string representation; redacts if requested.
func stringifyValue(fv reflect.Value, redact bool) string {
	if redact {
		return "<REDACTED>"
	}
	if !fv.IsValid() {
		return ""
	}
	if fv.Type() == reflect.TypeOf(time.Duration(0)) {
		return time.Duration(fv.Int()).String()
	}
	switch fv.Kind() {
	case reflect.String:
		return fv.String()
	case reflect.Bool:
		return strconv.FormatBool(fv.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(fv.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(fv.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(fv.Float(), 'f', -1, 64)
	case reflect.Slice:
		return fmt.Sprintf("%v", fv.Interface())
	default:
		return fmt.Sprintf("%v", fv.Interface())
	}
}
