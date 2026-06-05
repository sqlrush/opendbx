// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package skills

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func writeSkill(t *testing.T, path, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: a skill\n---\nbody\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func root(dir string) SkillRoot { return SkillRoot{Kind: SourceProject, Precedence: 1, Dir: dir} }

func TestScanRoot_FlatAndDirForm(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeSkill(t, filepath.Join(dir, "flat.md"), "flat")
	writeSkill(t, filepath.Join(dir, "dirform", "SKILL.md"), "dirform")
	// a non-.md file is ignored.
	_ = os.WriteFile(filepath.Join(dir, "README.txt"), []byte("x"), 0o644)
	// a subdir without SKILL.md is ignored.
	_ = os.MkdirAll(filepath.Join(dir, "empty"), 0o755)

	files, err := scanRoot(root(dir), 1000)
	if err != nil {
		t.Fatalf("scanRoot: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("found %d files; want 2 (%v)", len(files), files)
	}
	bases := map[string]bool{filepath.Base(files[0]): true, filepath.Base(files[1]): true}
	if !bases["flat.md"] || !bases["SKILL.md"] {
		t.Errorf("expected flat.md + dir-form SKILL.md, got %v", files)
	}
}

func TestScanRoot_MissingOptionalIsEmpty(t *testing.T) {
	t.Parallel()
	files, err := scanRoot(SkillRoot{Dir: filepath.Join(t.TempDir(), "does-not-exist")}, 1000)
	if err != nil || files != nil {
		t.Errorf("absent optional root should be empty, got files=%v err=%v", files, err)
	}
}

func TestScanRoot_MissingRequiredErrors(t *testing.T) {
	t.Parallel()
	_, err := scanRoot(SkillRoot{Dir: filepath.Join(t.TempDir(), "nope"), Required: true}, 1000)
	assertCode(t, err, ErrRootUnreadable.Code())
}

func TestScanRoot_RootIsFile(t *testing.T) {
	t.Parallel()
	f := filepath.Join(t.TempDir(), "afile")
	_ = os.WriteFile(f, []byte("x"), 0o644)
	_, err := scanRoot(root(f), 1000)
	assertCode(t, err, ErrRootUnreadable.Code())
}

func TestScanRoot_SymlinkRootRejected(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	target := t.TempDir()
	writeSkill(t, filepath.Join(target, "x.md"), "x")
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	_, err := scanRoot(root(link), 1000)
	assertCode(t, err, ErrRootUnreadable.Code())
}

func TestScanRoot_SymlinkEntrySkipped(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	dir := t.TempDir()
	writeSkill(t, filepath.Join(dir, "real.md"), "real")
	other := filepath.Join(t.TempDir(), "other.md")
	writeSkill(t, other, "other")
	if err := os.Symlink(other, filepath.Join(dir, "linked.md")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	files, err := scanRoot(root(dir), 1000)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || filepath.Base(files[0]) != "real.md" {
		t.Errorf("symlinked entry should be skipped, got %v", files)
	}
}

func TestScanRoot_MaxFilesTruncates(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, n := range []string{"a", "b", "c", "d"} {
		writeSkill(t, filepath.Join(dir, n+".md"), n)
	}
	files, err := scanRoot(root(dir), 2)
	if !errors.Is(err, ErrRootTooManyFiles) {
		t.Fatalf("want ROOT_TOO_MANY_FILES, got %v", err)
	}
	if len(files) != 2 {
		t.Errorf("truncated to %d; want 2", len(files))
	}
}
