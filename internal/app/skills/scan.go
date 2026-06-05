// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File scan.go — directory scanning for skill files (spec-2.2 D-2). Pure I/O,
// no parsing. Filesystem-safety is the focus:
//   - Lstat(root.Dir): reject a symlink/non-dir root; an absent OPTIONAL root
//     is empty (not an error) — a fresh install has no .claude/.opendbx dirs;
//     only an absent REQUIRED root (explicit skill_search_paths) errors.
//   - entry-level symlinks are skipped (no following).
//   - MaxFilesPerRoot truncates + reports SKILL.ROOT_TOO_MANY_FILES.
// The 1 MiB size cap is enforced in discovery.go BEFORE reading (stat-then-read).

package skills

import (
	"os"
	"path/filepath"
	"sort"
)

const skillFileName = "SKILL.md"

// scanRoot lists the SKILL.md files under one root: flat-form `<name>.md`
// directly in the root, and dir-form `<name>/SKILL.md` one level down. It does
// not read file contents. When err is ROOT_TOO_MANY_FILES the returned files
// are the (truncated) prefix and the caller still processes them.
func scanRoot(root SkillRoot, maxFiles int) (files []string, err error) {
	info, lerr := os.Lstat(root.Dir)
	if lerr != nil {
		if os.IsNotExist(lerr) {
			if root.Required {
				return nil, rootUnreadable(lerr)
			}
			return nil, nil // optional root absent → empty, not an error
		}
		return nil, rootUnreadable(lerr)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, rootUnreadable(&os.PathError{Op: "scan", Path: root.Dir, Err: errSymlinkRoot})
	}
	if !info.IsDir() {
		return nil, rootUnreadable(&os.PathError{Op: "scan", Path: root.Dir, Err: errNotDir})
	}

	entries, rerr := os.ReadDir(root.Dir)
	if rerr != nil {
		return nil, rootUnreadable(rerr)
	}

	for _, e := range entries {
		if e.Type()&os.ModeSymlink != 0 {
			continue // never follow symlinked entries
		}
		switch {
		case e.IsDir():
			// dir-form: <name>/SKILL.md (regular, non-symlink).
			p := filepath.Join(root.Dir, e.Name(), skillFileName)
			if isRegularFile(p) {
				files = append(files, p)
			}
		case filepath.Ext(e.Name()) == ".md":
			files = append(files, filepath.Join(root.Dir, e.Name()))
		}
	}

	sort.Strings(files)

	if maxFiles > 0 && len(files) > maxFiles {
		return files[:maxFiles], errcodeTooManyFiles(root.Dir, len(files), maxFiles)
	}
	return files, nil
}

// isRegularFile reports whether p exists and is a regular, non-symlink file.
func isRegularFile(p string) bool {
	info, err := os.Lstat(p)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}
