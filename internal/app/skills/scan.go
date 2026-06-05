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
	"io"
	"os"
	"path/filepath"
	"sort"
)

const skillFileName = "SKILL.md"

// scanBatch is how many directory entries are read per ReadDir call. Streaming
// in batches bounds memory on a pathological directory (a million-entry dir is
// never materialized at once; review HIGH: ReadDir OOM guard).
const scanBatch = 256

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

	d, oerr := os.Open(root.Dir)
	if oerr != nil {
		return nil, rootUnreadable(oerr)
	}
	defer func() { _ = d.Close() }()

	// Stream entries in batches so a huge directory is never fully materialized;
	// stop early once maxFiles skill files have been collected.
	overLimit := false
	for !overLimit {
		batch, rerr := d.ReadDir(scanBatch)
		for _, e := range batch {
			p, ok := classifyEntry(root.Dir, e)
			if !ok {
				continue
			}
			if maxFiles > 0 && len(files) >= maxFiles {
				overLimit = true
				break
			}
			files = append(files, p)
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return files, rootUnreadable(rerr)
		}
	}

	sort.Strings(files)
	if overLimit {
		return files, errcodeTooManyFiles(root.Dir, maxFiles, maxFiles)
	}
	return files, nil
}

// classifyEntry maps a directory entry to a skill file path, or (._, false) if
// it is not a skill file. Symlinks and non-regular files are rejected: the
// d_type symlink check is a fast path, and isRegularFile (Lstat-based) is the
// reliable backstop for filesystems that return DT_UNKNOWN.
func classifyEntry(dir string, e os.DirEntry) (string, bool) {
	if e.Type()&os.ModeSymlink != 0 {
		return "", false // never follow symlinked entries
	}
	switch {
	case e.IsDir():
		p := filepath.Join(dir, e.Name(), skillFileName)
		if isRegularFile(p) {
			return p, true
		}
	case filepath.Ext(e.Name()) == ".md":
		p := filepath.Join(dir, e.Name())
		if isRegularFile(p) { // backstop: rejects DT_UNKNOWN symlinks / FIFO / device
			return p, true
		}
	}
	return "", false
}

// isRegularFile reports whether p exists and is a regular, non-symlink file.
func isRegularFile(p string) bool {
	info, err := os.Lstat(p)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}
