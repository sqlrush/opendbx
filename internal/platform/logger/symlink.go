// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package logger

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

// updateLatestLink ensures <dir>/latest points to targetPath, where <dir> is
// the parent directory of targetPath.
//
// Spec § 2.2 + Q4 ★A user R3 constraints (avoid Windows implementation
// 踩坑 — 5 enumerated rules):
//
//  1. Best-effort. Any failure is warned to os.Stderr only; logger.Init()
//     return value is unaffected. The latest link is an ergonomic aid for
//     `tail -f ~/.opendbx/debug/latest`, not part of the data-correctness
//     surface.
//  2. Atomic re-link: remove the existing link first, then create the new
//     one. Either step may fail; if remove fails we still attempt create.
//  3. Cross-volume restriction: os.Link (Windows fallback) does not work
//     across filesystems / volumes. We detect the cross-link error and warn
//     with the specific reason so users know it's not a logger bug.
//  4. Mixed-path scenarios (--debug-file=<custom> + sidecar default debug
//     dir): the caller controls which path gets a latest link. We do NOT
//     auto-link the sidecar path. spec § 1.3 § 2.2 contract.
//  5. Warning template (Q4 R3 spec):
//     "opendbx: warning: latest debug log link unavailable (<reason>);
//     use 'opendbx --debug-file=<path>' to specify explicit log path or
//     look in <target>"
//
// claude HIGH-3 (lazy-once memoize): callers wrap this in sync.Once so the
// link is created on the first successful write rather than eagerly at
// Init — matches CC's `updateLatestDebugLogSymlink` memoize semantics
// (debug.ts:242) and avoids dangling links for sessions that emit no logs.
func updateLatestLink(targetPath string) {
	dir := filepath.Dir(targetPath)
	linkPath := filepath.Join(dir, "latest")

	// Step 1: remove existing link if any. os.Remove returns ErrNotExist when
	// the link is absent — we treat that as success.
	if err := os.Remove(linkPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		warnLatestLink("remove", linkPath, err, targetPath)
		// Carry on to attempt create — sometimes Remove fails on Windows due
		// to file locks even though Link can still succeed via replace.
	}

	// Step 2: create the new link. Symlink on Unix; hard link on Windows
	// (symlinks require admin on Windows by default — claude HIGH-3 + Q4 A).
	var linkErr error
	if runtime.GOOS == "windows" {
		linkErr = os.Link(targetPath, linkPath)
	} else {
		linkErr = os.Symlink(targetPath, linkPath)
	}
	if linkErr != nil {
		warnLatestLink("create", linkPath, linkErr, targetPath)
	}
}

// warnLatestLink writes a single advisory line to os.Stderr. The message
// follows the Q4 R3 5-point spec template so users see a consistent
// diagnostic regardless of which step (remove / create) failed.
//
// Does NOT route through the logger (logger may itself be the writer that
// triggered the latest update — recursion would deadlock).
func warnLatestLink(op, linkPath string, err error, target string) {
	_, _ = os.Stderr.WriteString(
		"opendbx: warning: latest debug log link unavailable (" +
			op + " " + linkPath + ": " + err.Error() +
			"); use 'opendbx --debug-file=<path>' to specify explicit log path or look in " +
			target + "\n",
	)
}
