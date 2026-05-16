// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

//go:build spike
// +build spike

package spike

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFixtureRejectsInvalidEnum(t *testing.T) {
	t.Parallel()
	path := writeFixtureForTest(t, `{
	  "name": "bad-enum",
	  "cc_commit": "test",
	  "sources": [{"path": "src/ink/components/Box.tsx"}],
	  "artifact": "unit-test",
	  "viewport": {"width": 10, "height": 1},
	  "root": {"label": "root", "direction": "sideways"},
	  "expected": {"root": {"x": 0, "y": 0, "w": 10, "h": 1}}
	}`)
	_, err := LoadFixture(path)
	if err == nil || !strings.Contains(err.Error(), "invalid direction") {
		t.Fatalf("LoadFixture invalid direction err = %v, want invalid direction", err)
	}
}

func TestLoadFixtureRejectsInvalidBasisMode(t *testing.T) {
	t.Parallel()
	path := writeFixtureForTest(t, `{
	  "name": "bad-basis",
	  "cc_commit": "test",
	  "sources": [{"path": "src/ink/components/Box.tsx"}],
	  "artifact": "unit-test",
	  "viewport": {"width": 10, "height": 1},
	  "root": {"label": "root", "basis_mode": "fluid"},
	  "expected": {"root": {"x": 0, "y": 0, "w": 10, "h": 1}}
	}`)
	_, err := LoadFixture(path)
	if err == nil || !strings.Contains(err.Error(), "invalid basis_mode") {
		t.Fatalf("LoadFixture invalid basis mode err = %v, want invalid basis_mode", err)
	}
}

func TestLoadFixtureRejectsMissingRoot(t *testing.T) {
	t.Parallel()
	path := writeFixtureForTest(t, `{
	  "name": "missing-root",
	  "cc_commit": "test",
	  "sources": [{"path": "src/ink/components/Box.tsx"}],
	  "artifact": "unit-test",
	  "viewport": {"width": 10, "height": 1},
	  "expected": {"root": {"x": 0, "y": 0, "w": 10, "h": 1}}
	}`)
	_, err := LoadFixture(path)
	if err == nil || !strings.Contains(err.Error(), "root is required") {
		t.Fatalf("LoadFixture missing root err = %v, want root is required", err)
	}
}

func TestLoadFixtureRejectsMissingProvenance(t *testing.T) {
	t.Parallel()
	path := writeFixtureForTest(t, `{
	  "name": "missing-provenance",
	  "viewport": {"width": 10, "height": 1},
	  "root": {"label": "root"},
	  "expected": {"root": {"x": 0, "y": 0, "w": 10, "h": 1}}
	}`)
	_, err := LoadFixture(path)
	if err == nil || !strings.Contains(err.Error(), "cc_commit is required") {
		t.Fatalf("LoadFixture missing provenance err = %v, want cc_commit is required", err)
	}
}

func TestBuildTreeRejectsNegativeFlexValues(t *testing.T) {
	t.Parallel()
	root := &FixtureNode{Label: "root", Grow: -1}
	_, _, err := root.BuildTree()
	if err == nil || !strings.Contains(err.Error(), "grow must be >= 0") {
		t.Fatalf("BuildTree negative grow err = %v, want grow validation", err)
	}
}

func TestBuildTreeRejectsDuplicateLabels(t *testing.T) {
	t.Parallel()
	root := &FixtureNode{
		Label: "dup",
		Children: []*FixtureNode{
			{Label: "dup"},
		},
	}
	_, _, err := root.BuildTree()
	if err == nil || !strings.Contains(err.Error(), "duplicate label") {
		t.Fatalf("BuildTree duplicate labels err = %v, want duplicate label", err)
	}
}

func writeFixtureForTest(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}
