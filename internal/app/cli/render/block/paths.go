// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File paths.go — getDisplayPath helper for spec-1.10 D-3 hint row.
// Mirrors CC `src/utils/file.ts:155-170` getDisplayPath (B-25):
// CWD-relative > tilde-relative > absolute path ladder. R2 HIGH-2 /
// R4 MED-1: only use cwd-relative when rel does not start with `..`
// or `../` (i.e., the target is within CWD); otherwise fall back to
// home-relative `~/...`; otherwise return absolute path unchanged.

package block

import (
	"os"
	"path/filepath"
	"strings"
)

// getDisplayPath returns a human-friendly display form for a file path
// per CC getDisplayPath (B-25 file.ts:155-170). Resolution order:
//
//  1. Absolute path: try CWD-relative via filepath.Rel; accept only
//     when rel does NOT start with `..` or `../` (mirrors CC
//     `!relativePath.startsWith('..')` guard per R4 MED-1).
//  2. If CWD-rel fails or escapes CWD: try home-relative via
//     os.UserHomeDir; accept when path is within home dir.
//  3. Otherwise: return path unchanged (absolute).
//
// Empty / already-relative input returns input as-is.
func getDisplayPath(p string) string {
	if p == "" {
		return ""
	}
	if !filepath.IsAbs(p) {
		// Already relative — return as-is. CC also returns user-provided
		// relative paths unchanged.
		return p
	}

	if cwd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(cwd, p); err == nil {
			// R4 MED-1 guard: only accept when rel does not escape CWD.
			// Use OS separator (R5 NIT-1: claude path 1/3 caught literal
			// `..\\` matched 2-char double-backslash, never produced by
			// filepath.Rel; correct guard is OS-native separator).
			parentEscape := ".." + string(filepath.Separator)
			if rel != ".." && !strings.HasPrefix(rel, parentEscape) {
				return rel
			}
		}
	}

	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if p == home {
			return "~"
		}
		if strings.HasPrefix(p, home+string(filepath.Separator)) {
			return "~" + p[len(home):]
		}
	}

	return p
}
