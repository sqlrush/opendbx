// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSecretExposeCallsiteConfined enforces spec-1.19 D-8: the ONLY production
// (non-test) call site of SecretDSN.Expose() is this package's connection.go
// (the single db.Open boundary). A new Expose() call anywhere else in
// production code is a secret-leak regression and fails this test.
func TestSecretExposeCallsiteConfined(t *testing.T) {
	root, err := filepath.Abs("../..") // repo root (cwd is internal/bootstrap) — scans the WHOLE tree, stronger than just internal/
	if err != nil {
		t.Fatal(err)
	}
	const allowed = "internal/bootstrap/connection.go"
	var offenders []string

	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !strings.Contains(string(src), ".Expose()") {
			return nil
		}
		rel, _ := filepath.Rel(filepath.Dir(filepath.Dir(root)), path) // repo-relative-ish
		if !strings.HasSuffix(filepath.ToSlash(path), allowed) {
			offenders = append(offenders, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(offenders) > 0 {
		t.Errorf("SecretDSN.Expose() called outside %s (spec-1.19 D-8 secret-leak guard):\n  %s",
			allowed, strings.Join(offenders, "\n  "))
	}
}
