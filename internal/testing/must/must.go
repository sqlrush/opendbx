// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package must

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

// File reads file at path, returning its contents. Fails fast on error.
func File(t testing.TB, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path) // #nosec G304 -- spec-0.11 D-2: test helper, caller-controlled path
	if err != nil {
		t.Fatalf("must.File(%q): %v", path, err)
		return nil
	}
	return data
}

// WriteTemp writes content to a freshly-created subpath of t.TempDir()
// and returns the absolute path. The temp dir is auto-cleaned by
// t.Cleanup. Use for one-shot fixtures that don't need explicit dir
// control or permissions.
func WriteTemp(t testing.TB, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("must.WriteTemp(%q): %v", name, err)
		return ""
	}
	return path
}

// WriteFile writes content to <dir>/<name> with permission 0o600 and
// returns the absolute path. Caller controls dir (typically t.TempDir
// or a subdirectory). Use when multiple files share a temp root or
// permission matters. Binary-safe (takes []byte).
//
// spec-0.11 R3 codex round-2 MED-1: D-2 API契约 incomplete in earlier
// drafts; R4 added the signature so retrofit callers (e.g.,
// tools/ci-protection-check writeFiles) have a typed equivalent.
func WriteFile(t testing.TB, dir, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("must.WriteFile(%q): %v", path, err)
		return ""
	}
	return path
}

// JSON unmarshals raw into v or fails. Reports both the path-style
// destination type and the raw bytes prefix on failure.
func JSON(t testing.TB, raw []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(raw, v); err != nil {
		t.Fatalf("must.JSON: %v (first 80 bytes: %q)", err, firstN(raw, 80))
	}
}

// NoErr fails if err is non-nil. If err implements errcode.Error, the
// Code/Message/Hint chain is dumped to make remediation immediate.
func NoErr(t testing.TB, err error) {
	t.Helper()
	if err == nil {
		return
	}
	var ec errcode.Error
	if errors.As(err, &ec) {
		t.Fatalf("must.NoErr: %s: %s (hint: %s)",
			ec.Code(), ec.Message(), ec.Hint())
		return
	}
	t.Fatalf("must.NoErr: %v", err)
}

// ErrCode asserts err is non-nil AND implements errcode.Error with
// Code() == wantCode. Fails on mismatch with full chain dump.
func ErrCode(t testing.TB, err error, wantCode string) {
	t.Helper()
	if err == nil {
		t.Fatalf("must.ErrCode(%q): got nil error, want errcode.Error", wantCode)
		return
	}
	var ec errcode.Error
	if !errors.As(err, &ec) {
		t.Fatalf("must.ErrCode(%q): err %v is not an errcode.Error", wantCode, err)
		return
	}
	if ec.Code() != wantCode {
		t.Fatalf("must.ErrCode: got code %q, want %q (message=%q hint=%q)",
			ec.Code(), wantCode, ec.Message(), ec.Hint())
	}
}

// firstN returns the first n bytes of b (or all of b if shorter).
func firstN(b []byte, n int) []byte {
	if len(b) <= n {
		return b
	}
	return b[:n]
}
