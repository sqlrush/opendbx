// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File statusinfo.go — spec-1.25 D-4 status-bar info helpers. cwd
// abbreviation + git branch detection, read ONCE at startup by the
// interactive bootstrap (NOT on the render hot path). Stdlib only — no
// shell, no go-git dependency (CLAUDE.md 规则 6; Q2 ★A). git dirty state is
// out of scope for this spec (spec-1.25 §1.1 #7).

package llmapp

import (
	"os"
	"path/filepath"
	"strings"
)

// AbbrevCwd returns a compact display form of dir for the status bar:
// "~/sub/path" when dir is under home, "~" for home itself, otherwise the
// basename. Empty dir → "". spec-1.25 D-4 (NIT: exact cwd formatting).
func AbbrevCwd(dir, home string) string {
	if dir == "" {
		return ""
	}
	if home != "" {
		if dir == home {
			return "~"
		}
		if rel := strings.TrimPrefix(dir, home+string(os.PathSeparator)); rel != dir {
			return "~" + string(os.PathSeparator) + rel
		}
	}
	return filepath.Base(dir)
}

// GitBranch returns the current branch name of the git repository
// containing dir, or "" when dir is not in a repo, HEAD is detached, or any
// read fails. It walks up from dir to locate ".git" (a directory for a
// normal repo, or a file "gitdir: <path>" for a worktree/submodule), reads
// HEAD, and extracts the branch from "ref: refs/heads/<branch>". No shell,
// no fork, no dependency (spec-1.25 D-4 / Q2 ★A). Intended to be called
// once at startup; never on the render path (R-3).
func GitBranch(dir string) string {
	gitPath := findGitPath(dir)
	if gitPath == "" {
		return ""
	}
	gitDir := resolveGitDir(gitPath)
	if gitDir == "" {
		return ""
	}
	head, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(head))
	const refPrefix = "ref: refs/heads/"
	if !strings.HasPrefix(line, refPrefix) {
		return "" // detached HEAD (raw SHA) or unexpected form
	}
	return strings.TrimPrefix(line, refPrefix)
}

// findGitPath walks up from dir looking for a ".git" entry (dir or file).
// Returns the path to it, or "" if none is found before the filesystem root.
func findGitPath(dir string) string {
	if dir == "" {
		return ""
	}
	cur := dir
	for {
		candidate := filepath.Join(cur, ".git")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "" // reached filesystem root
		}
		cur = parent
	}
}

// resolveGitDir returns the git directory for gitPath. When gitPath is a
// directory it is the git dir; when it is a file ("gitdir: <path>"), the
// referenced path is returned (worktree/submodule form).
func resolveGitDir(gitPath string) string {
	info, err := os.Stat(gitPath)
	if err != nil {
		return ""
	}
	if info.IsDir() {
		return gitPath
	}
	data, err := os.ReadFile(gitPath)
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(data))
	const prefix = "gitdir: "
	if !strings.HasPrefix(line, prefix) {
		return ""
	}
	gitDir := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(filepath.Dir(gitPath), gitDir)
	}
	return gitDir
}
