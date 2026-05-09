// Copyright 2026 opendbx contributors. See LICENSE.
//
// Filesystem-pass checks (spec § 3.2): doc.go presence + pkg/ empty.
//
// Author: sqlrush
package rules

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

// CheckFilesystem walks the opendbx repo root and returns violations:
//
//  1. Every internal/ subdirectory containing at least one .go file MUST
//     contain a doc.go file.
//  2. Under pkg/, no .go files allowed in spec-0.2 (only README.md).
//
// External directories like .git/, vendor/, node_modules/, testdata/
// are skipped. Test files (*_test.go) do NOT count toward the doc.go
// presence requirement (a directory with only *_test.go is not a Go
// package and need not have doc.go).
func CheckFilesystem(repoRoot string) ([]string, error) {
	var violations []string

	internalRoot := filepath.Join(repoRoot, "internal")
	pkgRoot := filepath.Join(repoRoot, "pkg")

	// internal/ doc.go presence
	dirs, err := goDirsUnder(internalRoot)
	if err != nil {
		return nil, fmt.Errorf("walk internal/: %w", err)
	}
	for _, d := range dirs {
		if !d.hasNonTestGo {
			continue
		}
		if !d.hasDocGo {
			rel, _ := filepath.Rel(repoRoot, d.path)
			violations = append(violations, fmt.Sprintf("missing doc.go: %s (every Go package directory under internal/ must have doc.go per spec § 3.2 / D-7 rule 5)", rel))
		}
	}

	// pkg/ empty enforcement
	pkgDirs, err := goDirsUnder(pkgRoot)
	if err != nil {
		// pkg/ may not exist; that's fine.
		if _, statErr := filepath.Glob(pkgRoot); statErr == nil {
			return nil, fmt.Errorf("walk pkg/: %w", err)
		}
	}
	for _, d := range pkgDirs {
		if d.hasGoAny {
			rel, _ := filepath.Rel(repoRoot, d.path)
			violations = append(violations, fmt.Sprintf("pkg/ contains .go file: %s — spec § 1.1 D-9 keeps pkg/ empty until a public-API spec lands", rel))
		}
	}
	return violations, nil
}

type goDir struct {
	path         string
	hasGoAny     bool // any *.go file (incl. test)
	hasNonTestGo bool // any *.go file that is not *_test.go
	hasDocGo     bool // doc.go specifically
}

func goDirsUnder(root string) ([]goDir, error) {
	dirIndex := map[string]*goDir{}

	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// e.g. root doesn't exist — treat as no violations.
			if d == nil {
				return nil
			}
			return err
		}
		// Skip well-known non-source dirs.
		if d.IsDir() {
			name := d.Name()
			switch name {
			case ".git", "vendor", "node_modules", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		dir := filepath.Dir(path)
		gd, ok := dirIndex[dir]
		if !ok {
			gd = &goDir{path: dir}
			dirIndex[dir] = gd
		}
		gd.hasGoAny = true
		isTest := strings.HasSuffix(d.Name(), "_test.go")
		if !isTest {
			gd.hasNonTestGo = true
		}
		if d.Name() == "doc.go" {
			gd.hasDocGo = true
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	out := make([]goDir, 0, len(dirIndex))
	for _, gd := range dirIndex {
		out = append(out, *gd)
	}
	return out, nil
}
