// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package report

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func redirectReportsDir(t *testing.T, dir string) {
	t.Helper()
	old := reportsDirFn
	reportsDirFn = func() (string, error) { return dir, nil }
	t.Cleanup(func() { reportsDirFn = old })
}

func TestWriteReport_Success(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "reports")
	redirectReportsDir(t, dir)

	now := time.Date(2026, 5, 31, 12, 30, 45, 0, time.UTC)
	path, err := WriteReport("# 报告\n内容", now)
	if err != nil {
		t.Fatalf("WriteReport: %v", err)
	}
	// colon-safe UTC filename.
	if base := filepath.Base(path); base != "2026-05-31T12-30-45Z.md" {
		t.Errorf("filename = %q; want 2026-05-31T12-30-45Z.md", base)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("readback: %v", err)
	}
	if string(got) != "# 报告\n内容" {
		t.Errorf("content = %q; want round-trip", got)
	}
	// no leftover temp files.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}
}

// TestWriteReport_NonUTCNow_Normalized — a non-UTC instant is rendered in UTC
// so the filename is unambiguous.
func TestWriteReport_NonUTCNow_Normalized(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "reports")
	redirectReportsDir(t, dir)
	loc := time.FixedZone("UTC+8", 8*3600)
	now := time.Date(2026, 5, 31, 20, 30, 45, 0, loc) // == 12:30:45 UTC
	path, err := WriteReport("x", now)
	if err != nil {
		t.Fatalf("WriteReport: %v", err)
	}
	if base := filepath.Base(path); base != "2026-05-31T12-30-45Z.md" {
		t.Errorf("filename = %q; want UTC-normalized 12-30-45", base)
	}
}

// TestDefaultReportsDir resolves the real platform reports dir (config
// source-path logic); it must be non-empty and end in "reports".
func TestDefaultReportsDir(t *testing.T) {
	t.Parallel()
	dir, err := defaultReportsDir()
	if err != nil {
		t.Fatalf("defaultReportsDir: %v", err)
	}
	if dir == "" || filepath.Base(dir) != "reports" {
		t.Errorf("reports dir = %q; want a path ending in /reports", dir)
	}
}

// TestWriteReport_RenameFails_Errcode — the destination path already exists as a
// directory, so the final rename fails → REPORT.WRITE_FAILED.
func TestWriteReport_RenameFails_Errcode(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "reports")
	redirectReportsDir(t, dir)
	now := time.Date(2026, 5, 31, 1, 2, 3, 0, time.UTC)
	// pre-create the target as a directory so os.Rename(file→dir) fails.
	dest := filepath.Join(dir, "2026-05-31T01-02-03Z.md")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if _, err := WriteReport("x", now); !errors.Is(err, ErrWriteFailed) {
		t.Errorf("want ErrWriteFailed on rename clash; got %v", err)
	}
}

func TestWriteReport_DirResolveError_Errcode(t *testing.T) {
	old := reportsDirFn
	reportsDirFn = func() (string, error) { return "", errors.New("resolve boom") }
	t.Cleanup(func() { reportsDirFn = old })
	_, err := WriteReport("x", time.Now())
	if !errors.Is(err, ErrWriteFailed) {
		t.Errorf("want ErrWriteFailed; got %v", err)
	}
}

// TestWriteReport_MkdirFails_Errcode — reports dir nested under a regular file
// makes MkdirAll fail; the error must be REPORT.WRITE_FAILED.
func TestWriteReport_MkdirFails_Errcode(t *testing.T) {
	base := t.TempDir()
	file := filepath.Join(base, "afile")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	old := reportsDirFn
	reportsDirFn = func() (string, error) { return filepath.Join(file, "reports"), nil } // under a file
	t.Cleanup(func() { reportsDirFn = old })
	_, err := WriteReport("x", time.Now())
	if !errors.Is(err, ErrWriteFailed) {
		t.Errorf("want ErrWriteFailed; got %v", err)
	}
}
