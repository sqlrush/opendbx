// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File write.go — persist a report to disk (spec-1.23 D-5). The write is atomic
// (temp file + rename) and resolves the reports directory cross-platform via
// the same config source-path logic (macOS Application Support / Linux XDG /
// Windows APPDATA, with the ~/.opendbx fallback) rather than hardcoding a path.
//
// This file does plain blocking IO; the llmapp Model invokes WriteReport from a
// scheduler.Cmd (NOT from the pure Update path) so the architecture's
// Update-is-pure contract holds.

package report

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sqlrush/opendbx/internal/platform/config"
	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

// ErrWriteFailed surfaces an IO failure while persisting a report (规则 7
// Code/Message/Hint). It is an output-layer IO error, not a degraded
// diagnosis, so it does not violate 原则 3 (the report content is still
// rendered to the TUI; only the file persistence failed).
var ErrWriteFailed = errcode.Register(
	"REPORT.WRITE_FAILED",
	"故障报告写入失败",
	"检查 opendbx reports 目录的写权限与磁盘空间",
)

// reportsDirFn resolves the reports directory. It is a package seam so tests
// can redirect writes to a temp dir. Guarded by reportsDirMu so the test-seam
// mutation is visible to -race even if a test ever runs in parallel
// (spec-1.23 R-fix; post-impl go-reviewer MED-1).
var (
	reportsDirMu sync.RWMutex
	reportsDirFn = defaultReportsDir
)

// resolveReportsDir reads the (possibly test-redirected) reports dir func
// under the lock.
func resolveReportsDir() (string, error) {
	reportsDirMu.RLock()
	fn := reportsDirFn
	reportsDirMu.RUnlock()
	return fn()
}

// WriteReport atomically writes md to <reports-dir>/<UTC-ts>.md and returns the
// path. now must be the same instant used to render the report header so the
// filename and header agree (it is rendered in UTC; the colons of RFC3339 are
// replaced with hyphens for filename safety).
func WriteReport(md string, now time.Time) (string, error) {
	dir, err := resolveReportsDir()
	if err != nil {
		return "", errcode.Wrap(ErrWriteFailed.Code(), err, "resolve reports directory", "")
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", errcode.Wrap(ErrWriteFailed.Code(), err, "create reports directory", "")
	}
	name := now.UTC().Format("2006-01-02T15-04-05Z") + ".md"
	path := filepath.Join(dir, name)

	tmp, err := os.CreateTemp(dir, ".report-*.tmp")
	if err != nil {
		return "", errcode.Wrap(ErrWriteFailed.Code(), err, "create temp report file", "")
	}
	tmpName := tmp.Name()
	if _, err := tmp.WriteString(md); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return "", errcode.Wrap(ErrWriteFailed.Code(), err, "write report contents", "")
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return "", errcode.Wrap(ErrWriteFailed.Code(), err, "close report file", "")
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return "", errcode.Wrap(ErrWriteFailed.Code(), err, "finalize report file", "")
	}
	return path, nil
}

// defaultReportsDir = <opendbx-config-dir>/reports, reusing the config
// source-path resolution so it honours the platform conventions.
func defaultReportsDir() (string, error) {
	sp, err := config.DefaultSourcePaths(".")
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(sp.UserPath), "reports"), nil
}
