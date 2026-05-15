// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package tablerun

import (
	"reflect"
	"testing"
)

// mustExtractName reflectively reads the required `Name string` field
// from any struct. Fails fast — no fmt.Sprintf fallback (spec-0.11
// R2 codex HIGH-2 + R3 codex round-2 HIGH-2: single contract, no
// silent-name fallback).
//
// Failure modes (each → t.Fatalf):
//   - case is not a struct
//   - struct lacks `Name` field
//   - `Name` is not a string
//   - `Name` is empty string
//
// idx is the position in the case slice (0-based) — included in the
// failure message so the offending case is identifiable.
//
// Accepts testing.TB rather than *testing.T so unit tests can inject a
// mock TB that records Fatalf without failing the outer test.
func mustExtractName(t testing.TB, idx int, c any) string {
	t.Helper()
	v := reflect.ValueOf(c)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		t.Fatalf("tablerun: case %d is %T, not a struct", idx, c)
		return "" // Fatalf halts real *testing.T via Goexit; explicit
		//          return guards against mock TB (test-only injection).
	}
	f := v.FieldByName("Name")
	if !f.IsValid() {
		t.Fatalf("tablerun: case %d (%T) missing required Name field", idx, c)
		return ""
	}
	if f.Kind() != reflect.String {
		t.Fatalf("tablerun: case %d Name field is %s, want string", idx, f.Kind())
		return ""
	}
	name := f.String()
	if name == "" {
		t.Fatalf("tablerun: case %d has empty Name string", idx)
		return ""
	}
	return name
}
