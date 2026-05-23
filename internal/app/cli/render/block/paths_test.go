// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File paths_test.go — spec-1.10 D-3 / T1-24 unit test for getDisplayPath
// (R5 absorb H-1/H-2: claude path 1/3 caught spec-mandated paths_test.go
// missing + `..` guard had 0% test coverage). Mirrors CC B-25 file.ts:
// 155-170 ladder semantics.

package block

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetDisplayPath_Empty(t *testing.T) {
	t.Parallel()
	if got := getDisplayPath(""); got != "" {
		t.Errorf("empty: want \"\", got %q", got)
	}
}

func TestGetDisplayPath_AlreadyRelative(t *testing.T) {
	t.Parallel()
	cases := []string{
		"foo.txt",
		"a/b/c",
		"./local",
		"../parent",
	}
	for _, p := range cases {
		if got := getDisplayPath(p); got != p {
			t.Errorf("relative input %q: want unchanged, got %q", p, got)
		}
	}
}

// TestGetDisplayPath_WithinCWD covers the cwd-relative branch (lines 44-46
// in paths.go) — the most common production scenario and 0%-covered in
// the original PR per claude H-1.
func TestGetDisplayPath_WithinCWD(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	abs := filepath.Join(cwd, "subdir", "file.txt")
	got := getDisplayPath(abs)
	want := filepath.Join("subdir", "file.txt")
	if got != want {
		t.Errorf("within cwd: want %q, got %q", want, got)
	}
}

// TestGetDisplayPath_CWDEscape covers the R4 MED-1 `..` guard — the most
// critical correctness requirement of D-3 and 0%-covered per claude H-2.
// A path one level above CWD must NOT be returned as cwd-relative.
func TestGetDisplayPath_CWDEscape(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	parent := filepath.Dir(cwd)
	got := getDisplayPath(parent)
	// Must NOT return a `..`-prefixed cwd-relative form (guard fires).
	if got == ".." || strings.HasPrefix(got, "..") {
		t.Errorf("parent escape must fall through cwd guard, got %q", got)
	}
	// Should fall through to home-rel or absolute (NOT cwd-rel).
	// Acceptable outcomes: "~..." OR absolute parent path unchanged.
	home, _ := os.UserHomeDir()
	homePrefixed := home != "" && (got == "~" || strings.HasPrefix(got, "~/"))
	absoluteUnchanged := got == parent
	if !homePrefixed && !absoluteUnchanged {
		t.Errorf("parent escape: want home-rel or absolute parent, got %q", got)
	}
}

func TestGetDisplayPath_HomeExact(t *testing.T) {
	t.Parallel()
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no HOME available")
	}
	if got := getDisplayPath(home); got != "~" {
		t.Errorf("home exact: want \"~\", got %q", got)
	}
}

func TestGetDisplayPath_HomeRelative(t *testing.T) {
	t.Parallel()
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no HOME available")
	}
	abs := filepath.Join(home, "Documents", "file.txt")
	// Only meaningful if home is not the current CWD (otherwise cwd-rel
	// path wins; that branch is covered by TestGetDisplayPath_WithinCWD).
	cwd, _ := os.Getwd()
	if rel, _ := filepath.Rel(cwd, abs); rel == "" || !strings.HasPrefix(rel, "..") {
		// Path is reachable from CWD — skip this case (covered elsewhere).
		t.Skip("home file is within CWD; covered by WithinCWD test")
	}
	got := getDisplayPath(abs)
	wantPrefix := "~" + string(filepath.Separator)
	if got == "~" || strings.HasPrefix(got, wantPrefix) {
		return
	}
	t.Errorf("home rel: want %s..., got %q", wantPrefix, got)
}

// TestGetDisplayPath_AbsoluteOutsideAll covers the final fall-through:
// not in CWD, not in HOME → absolute unchanged.
func TestGetDisplayPath_AbsoluteOutsideAll(t *testing.T) {
	t.Parallel()
	// /etc is reliably outside both CWD and $HOME on darwin/linux.
	p := "/etc/hosts"
	got := getDisplayPath(p)
	// Should be unchanged absolute (or rare: cwd-rel if test runs in /).
	cwd, _ := os.Getwd()
	if rel, err := filepath.Rel(cwd, p); err == nil && !strings.HasPrefix(rel, "..") {
		// CWD happens to contain /etc (e.g. running tests with CWD=/) —
		// cwd-rel is correct.
		if got != rel {
			t.Errorf("special CWD=/: want %q, got %q", rel, got)
		}
		return
	}
	home, _ := os.UserHomeDir()
	if home != "" && strings.HasPrefix(p, home+string(filepath.Separator)) {
		t.Skip("/etc happens to be under HOME (unusual layout)")
	}
	if got != p {
		t.Errorf("outside all: want %q unchanged, got %q", p, got)
	}
}
