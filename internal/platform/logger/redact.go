// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package logger

import (
	"reflect"
	"regexp"
	"strings"
)

// redactionToken is the canonical replacement string for secret-bearing
// values. Choosing a distinctive constant lets downstream consumers grep
// for accidental escapes ("did any path skip redaction?").
const redactionToken = "<REDACTED>"

// redactPatterns matches secret-bearing substrings inside free-form strings.
// Each entry has:
//   - re: regex matching the secret context (key=value form, header form, URL userinfo, etc.)
//   - mask: a callback that returns the redacted replacement for a given match
//
// The patterns deliberately mask only the SECRET PORTION while preserving the
// key / header / scheme prefix so debug consumers can still see "an
// Authorization header was present" without learning the token.
//
// codex MED-1 + claude MED-2 共识: covers password / token / key / Authorization
// / Bearer / sk-* / URL userinfo (user:pass@host).
var redactPatterns = []struct {
	re   *regexp.Regexp
	mask func(submatches []string) string
}{
	// key=value form (case-insensitive). Matches password / passwd / token /
	// api_key / apikey / secret / pwd. The value extends to whitespace, semi
	// colon, comma, ampersand, single/double quote, or end of string.
	{
		re: regexp.MustCompile(`(?i)(password|passwd|pwd|token|api[_-]?key|apikey|secret|access[_-]?key)=[^\s;,&"']+`),
		mask: func(m []string) string {
			return m[1] + "=" + redactionToken
		},
	},
	// Authorization: <scheme> <token> header form. Mask only the token.
	{
		re:   regexp.MustCompile(`(?i)(Authorization:\s*\S+\s+)\S+`),
		mask: func(m []string) string { return m[1] + redactionToken },
	},
	// Bearer <token> (anywhere — common in code samples / cURL traces).
	// Stop at quote / semicolon / comma so surrounding shell quoting is
	// preserved verbatim ("curl -H 'Bearer xyz'" → keep both single quotes).
	{
		re:   regexp.MustCompile(`(?i)Bearer\s+[^\s;,'"]+`),
		mask: func(_ []string) string { return "Bearer " + redactionToken },
	},
	// Anthropic-style sk-... API keys (40-100+ alphanumeric chars after sk-).
	{
		re:   regexp.MustCompile(`sk-[A-Za-z0-9_-]{16,}`),
		mask: func(_ []string) string { return "sk-" + redactionToken },
	},
	// URL userinfo: scheme://user:pass@host  →  scheme://user:<REDACTED>@host.
	{
		re:   regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://[^:/?#\s]+):[^@\s]+@`),
		mask: func(m []string) string { return m[1] + ":" + redactionToken + "@" },
	},
}

// redactString returns s with all known secret patterns masked. Idempotent
// (applying twice is harmless because <REDACTED> itself contains no
// secret-shaped substrings).
//
// spec § 2.6 post-format fail-safe pass: ALL logger output (main text +
// JSONL sidecar) goes through this before hitting BufferedWriter. This is
// belt-and-braces with the pre-format Attr redaction (redactAttrs / redactValue)
// so a caller that smuggled a raw secret into msg/attrs can't escape both.
func redactString(s string) string {
	out := s
	for _, p := range redactPatterns {
		out = p.re.ReplaceAllStringFunc(out, func(match string) string {
			sub := p.re.FindStringSubmatch(match)
			return p.mask(sub)
		})
	}
	return out
}

// redactAttrs returns a new slice with each Attr value transformed by
// redactValue. The input is not mutated (spec § 1 immutability rule from
// the global coding-style guide).
//
// codex MED-1 + claude MED-2: covers nested struct / map / slice values via
// reflection. Reserved keys (event / trace_id / span_id) are passed through
// unchanged because they are operator-supplied verbs / IDs, not secret data.
func redactAttrs(attrs []Attr) []Attr {
	if len(attrs) == 0 {
		return attrs
	}
	out := make([]Attr, len(attrs))
	for i, a := range attrs {
		if _, reserved := reservedAttrKeys[a.Key]; reserved {
			out[i] = a
			continue
		}
		out[i] = Attr{Key: a.Key, Value: redactValue(a.Value, secretKeyHint(a.Key))}
	}
	return out
}

// secretKeyHint returns true if the attr key itself suggests a secret value
// (e.g. "password", "apiKey"). When true, redactValue treats string values
// as secrets-by-name even if their content doesn't match the regex patterns.
func secretKeyHint(key string) bool {
	k := strings.ToLower(key)
	for _, marker := range []string{"password", "passwd", "pwd", "token", "secret", "apikey", "api_key", "api-key", "access_key", "accesskey", "authorization"} {
		if strings.Contains(k, marker) {
			return true
		}
	}
	return false
}

// redactValue recursively redacts a value. Behaviour by kind:
//
//   - string: if keyIsSecret, replace entirely with <REDACTED>; else
//     redactString (regex-masked).
//   - error: redactString on err.Error() and wrap in redactedError.
//   - struct: walk exported fields; honour `redact:"true"` field tag from
//     spec-0.4 D-6 contract — tagged fields replaced entirely. Untagged
//     fields recurse.
//   - map: recurse on values (keys are not secrets in our model).
//   - slice / array: recurse on elements.
//   - pointer / interface: recurse on element.
//   - everything else (int / bool / float / time / etc.): returned as-is.
//
// keyIsSecret carries the "the surrounding attr key looks like a secret"
// hint so a bare string Value gets fully redacted, not just regex-masked.
func redactValue(v any, keyIsSecret bool) any {
	if v == nil {
		return nil
	}
	switch x := v.(type) {
	case string:
		if keyIsSecret {
			return redactionToken
		}
		return redactString(x)
	case error:
		return redactedError{msg: redactString(x.Error())}
	}
	rv := reflect.ValueOf(v)
	return redactReflect(rv, keyIsSecret).Interface()
}

// redactReflect handles non-trivial kinds via reflection. Returns a new
// reflect.Value (not modifying the input) whose dynamic type matches the
// input. For unsupported kinds it returns the input unchanged.
//
// Type-preserving contract (codex CRIT-3 + claude HIGH-1 + go-reviewer MED-1
// integration): pointer fields stay pointers, fixed-size arrays stay arrays,
// slices stay slices. This is critical for redact-tag fields whose target
// type must match the source — earlier versions of this function unwrapped
// pointers and built mismatched slices, causing the assignability check to
// fail and silently copy the ORIGINAL (un-redacted) value back into the
// output struct.
func redactReflect(rv reflect.Value, keyIsSecret bool) reflect.Value {
	if !rv.IsValid() {
		return rv
	}
	switch rv.Kind() {
	case reflect.Pointer:
		if rv.IsNil() {
			return rv
		}
		// Recurse on the element, then re-wrap in a fresh pointer of the
		// same type so the redacted value lands back behind the original
		// pointer shape. Without this, `*string redact:"true"` leaks (the
		// caller's pointer field gets the original *string back).
		inner := redactReflect(rv.Elem(), keyIsSecret)
		if !inner.IsValid() {
			return rv
		}
		ptr := reflect.New(rv.Type().Elem())
		if inner.Type().AssignableTo(ptr.Elem().Type()) {
			ptr.Elem().Set(inner)
		} else {
			// Type drift (e.g. inner unwrapped through interface): fall back
			// to a copy of the original. This branch should not fire under
			// the new pointer-preserving rule but keeps the function safe.
			ptr.Elem().Set(rv.Elem())
		}
		return ptr
	case reflect.Interface:
		if rv.IsNil() {
			return rv
		}
		return redactReflect(rv.Elem(), keyIsSecret)
	case reflect.Struct:
		t := rv.Type()
		outPtr := reflect.New(t).Elem()
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			fieldVal := rv.Field(i)
			fieldRedact := f.Tag.Get("redact") == "true" || keyIsSecret || secretKeyHint(f.Name)
			if fieldRedact && fieldVal.Kind() == reflect.String {
				outPtr.Field(i).SetString(redactionToken)
				continue
			}
			// Pointer-to-string redact-tag escape: when the field is `*string`
			// and the tag flags it for redaction, replace the pointee with
			// the token directly rather than recursing (avoids the type
			// drift dance for the most common secret-field case).
			if fieldRedact && fieldVal.Kind() == reflect.Pointer &&
				fieldVal.Type().Elem().Kind() == reflect.String && !fieldVal.IsNil() {
				masked := redactionToken
				ptr := reflect.New(fieldVal.Type().Elem())
				ptr.Elem().SetString(masked)
				outPtr.Field(i).Set(ptr)
				continue
			}
			rec := redactReflect(fieldVal, fieldRedact)
			if rec.IsValid() && outPtr.Field(i).CanSet() && rec.Type().AssignableTo(outPtr.Field(i).Type()) {
				outPtr.Field(i).Set(rec)
			} else if outPtr.Field(i).CanSet() {
				outPtr.Field(i).Set(fieldVal)
			}
		}
		return outPtr
	case reflect.Map:
		if rv.IsNil() {
			return rv
		}
		out := reflect.MakeMapWithSize(rv.Type(), rv.Len())
		for it := rv.MapRange(); it.Next(); {
			key := it.Key()
			val := it.Value()
			subSecret := keyIsSecret
			if key.Kind() == reflect.String {
				subSecret = subSecret || secretKeyHint(key.String())
			}
			redacted := redactValueReflect(val, subSecret)
			// Coerce redacted to the map's element type so SetMapIndex does
			// not panic on assignability mismatch.
			if redacted.IsValid() && redacted.Type().AssignableTo(rv.Type().Elem()) {
				out.SetMapIndex(key, redacted)
			} else {
				out.SetMapIndex(key, val)
			}
		}
		return out
	case reflect.Slice:
		if rv.IsNil() {
			return rv
		}
		out := reflect.MakeSlice(rv.Type(), rv.Len(), rv.Len())
		for i := 0; i < rv.Len(); i++ {
			redacted := redactValueReflect(rv.Index(i), keyIsSecret)
			if redacted.IsValid() && redacted.Type().AssignableTo(rv.Type().Elem()) {
				out.Index(i).Set(redacted)
			} else {
				out.Index(i).Set(rv.Index(i))
			}
		}
		return out
	case reflect.Array:
		// Fixed-size arrays: type-preserving requires reflect.New(arrayType)
		// rather than MakeSlice. claude HIGH-1 + go-reviewer MED-1.
		out := reflect.New(rv.Type()).Elem()
		for i := 0; i < rv.Len(); i++ {
			redacted := redactValueReflect(rv.Index(i), keyIsSecret)
			if redacted.IsValid() && redacted.Type().AssignableTo(rv.Type().Elem()) {
				out.Index(i).Set(redacted)
			} else {
				out.Index(i).Set(rv.Index(i))
			}
		}
		return out
	case reflect.String:
		if keyIsSecret {
			return reflect.ValueOf(redactionToken)
		}
		return reflect.ValueOf(redactString(rv.String()))
	default:
		return rv
	}
}

// redactValueReflect is a reflect-wrapped redactValue for inner recursion
// from map / slice. Always returns a reflect.Value assignable to the
// container's element type.
func redactValueReflect(rv reflect.Value, keyIsSecret bool) reflect.Value {
	if !rv.IsValid() {
		return rv
	}
	// Unwrap interface{} so we recurse into the concrete element.
	if rv.Kind() == reflect.Interface && !rv.IsNil() {
		inner := redactValue(rv.Interface(), keyIsSecret)
		return reflect.ValueOf(inner)
	}
	rec := redactValue(rv.Interface(), keyIsSecret)
	return reflect.ValueOf(rec)
}

// redactedError wraps a redacted error message so logger error-emission
// paths see a clean error type rather than the original (which might still
// contain the secret if it's chained via fmt.Errorf %w).
type redactedError struct{ msg string }

func (e redactedError) Error() string { return e.msg }
