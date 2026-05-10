// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package config

import (
	"path/filepath"
	"testing"
)

func TestDefaultSourcePaths_NoCWD(t *testing.T) {
	p, err := DefaultSourcePaths("")
	if err != nil {
		t.Fatalf("DefaultSourcePaths: %v", err)
	}
	if p.UserPath == "" {
		t.Error("UserPath empty")
	}
	if p.ProjectPath != "" {
		t.Errorf("ProjectPath should be empty when CWD empty, got %q", p.ProjectPath)
	}
	if p.LocalPath != "" {
		t.Errorf("LocalPath should be empty when CWD empty, got %q", p.LocalPath)
	}
}

func TestDefaultSourcePaths_WithCWD(t *testing.T) {
	cwd := "/tmp/test-cwd"
	p, err := DefaultSourcePaths(cwd)
	if err != nil {
		t.Fatalf("DefaultSourcePaths: %v", err)
	}
	if p.ProjectPath != filepath.Join(cwd, ".opendbx", "config.yaml") {
		t.Errorf("ProjectPath = %q", p.ProjectPath)
	}
	if p.LocalPath != filepath.Join(cwd, ".opendbx", "local.yaml") {
		t.Errorf("LocalPath = %q", p.LocalPath)
	}
}

func TestFileExists(t *testing.T) {
	if fileExists("") {
		t.Error("fileExists(\"\") = true, want false")
	}
	if fileExists("/nonexistent/path/should/not/exist/file.yaml") {
		t.Error("fileExists on nonexistent = true")
	}
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "sub")
	if err := mkdir(dir); err != nil {
		t.Fatal(err)
	}
	if fileExists(dir) {
		t.Error("fileExists on directory = true (should be false)")
	}
	file := filepath.Join(tmp, "x.yaml")
	if err := writeFile(file, "x: 1"); err != nil {
		t.Fatal(err)
	}
	if !fileExists(file) {
		t.Error("fileExists on real file = false")
	}
}
