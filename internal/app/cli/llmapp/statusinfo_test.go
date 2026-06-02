// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package llmapp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAbbrevCwd(t *testing.T) {
	cases := []struct {
		name, dir, home, want string
	}{
		{"under home", "/Users/me/opendbx", "/Users/me", "~/opendbx"},
		{"home itself", "/Users/me", "/Users/me", "~"},
		{"deep under home", "/Users/me/a/b", "/Users/me", "~/a/b"},
		{"outside home", "/etc/conf", "/Users/me", "conf"},
		{"empty dir", "", "/Users/me", ""},
		{"empty home falls back to basename", "/var/db", "", "db"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := AbbrevCwd(c.dir, c.home); got != c.want {
				t.Errorf("AbbrevCwd(%q,%q) = %q; want %q", c.dir, c.home, got, c.want)
			}
		})
	}
}

func TestGitBranch_DirRepo(t *testing.T) {
	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(gitDir, "HEAD"), "ref: refs/heads/spec-1.25\n")
	// from a nested subdir, GitBranch walks up to find .git
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := GitBranch(sub); got != "spec-1.25" {
		t.Errorf("GitBranch = %q; want spec-1.25", got)
	}
}

func TestGitBranch_Detached(t *testing.T) {
	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	_ = os.MkdirAll(gitDir, 0o755)
	writeFile(t, filepath.Join(gitDir, "HEAD"), "9fceb02d0ae598e95dc970b74767f19372d61af8\n")
	if got := GitBranch(root); got != "" {
		t.Errorf("detached HEAD GitBranch = %q; want empty", got)
	}
}

func TestGitBranch_GitFileWorktree(t *testing.T) {
	root := t.TempDir()
	realGit := filepath.Join(root, "realgit")
	_ = os.MkdirAll(realGit, 0o755)
	writeFile(t, filepath.Join(realGit, "HEAD"), "ref: refs/heads/wt-branch\n")
	// .git as a FILE pointing at the real gitdir (worktree/submodule form)
	writeFile(t, filepath.Join(root, ".git"), "gitdir: "+realGit+"\n")
	if got := GitBranch(root); got != "wt-branch" {
		t.Errorf("worktree GitBranch = %q; want wt-branch", got)
	}
}

func TestGitBranch_NoRepo(t *testing.T) {
	if got := GitBranch(t.TempDir()); got != "" {
		t.Errorf("no-repo GitBranch = %q; want empty", got)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
