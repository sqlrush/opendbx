// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package layout

import (
	"path/filepath"
	"testing"
)

// sampleNames lists the 5 locked CC UI sample fixtures (spec-0.12.5
// D-2 → spec-1.1 D-5 promote-to-production). Array type [5]string is
// intentional: the spec § 2.1 "5 locked" rule is encoded into Go's
// type system. Adding / removing a sample requires editing the array
// length, which fails compilation and forces a spec errata.
var sampleNames = [5]string{
	"repl-prompt",
	"message-block",
	"tool-call-panel",
	"status-line",       // CRITICAL (Row + intrinsic + grow)
	"permission-picker", // CRITICAL (Column + intrinsic + shrink overflow)
}

// TestCCSamples runs the locked 5 CC UI sample fixtures against the
// production flex layouter. Each fixture has labeled nodes + expected
// Box positions; any divergence fails the test.
func TestCCSamples(t *testing.T) {
	t.Parallel()
	for _, name := range sampleNames {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fx, err := loadFixture(filepath.Join("testdata", "cc-samples", name+".json"))
			if err != nil {
				t.Fatalf("load fixture: %v", err)
			}
			root, index, err := fx.Root.buildTree()
			if err != nil {
				t.Fatalf("build tree: %v", err)
			}
			got, err := NewFlexLayouter().Layout(root, fx.Viewport.viewportBox())
			if err != nil {
				t.Fatalf("Layout err = %v", err)
			}
			for label := range index {
				if _, exists := fx.Expected[label]; !exists {
					t.Errorf("tree label %q has no expected box", label)
				}
			}
			for label, expected := range fx.Expected {
				node, exists := index[label]
				if !exists {
					t.Errorf("expected label %q not in tree", label)
					continue
				}
				if got[node] != expected.toBox() {
					t.Errorf("%s: got %+v, want %+v", label, got[node], expected.toBox())
				}
			}
		})
	}
}

// TestSampleLockedAt5 enforces the spec-0.12.5 § 2.1 / § 11.1 "5
// locked" rule at compile time via the [5]string array type.
func TestSampleLockedAt5(t *testing.T) {
	t.Parallel()
	if len(sampleNames) != 5 {
		t.Errorf("sample count = %d, want 5 (locked per spec-0.12.5 § 2.1)", len(sampleNames))
	}
}

// TestCriticalSamplesPresent asserts the two critical samples
// (status-line + permission-picker) are present and marked critical
// in their fixture JSON.
func TestCriticalSamplesPresent(t *testing.T) {
	t.Parallel()
	inSet := map[string]bool{"status-line": false, "permission-picker": false}
	for _, name := range sampleNames {
		fx, err := loadFixture(filepath.Join("testdata", "cc-samples", name+".json"))
		if err != nil {
			t.Fatalf("load fixture %s: %v", name, err)
		}
		if _, required := inSet[name]; required {
			if !fx.Critical {
				t.Errorf("%s: critical=false in fixture, expected true", name)
			}
			inSet[name] = true
		}
	}
	for name, found := range inSet {
		if !found {
			t.Errorf("required critical sample %q missing from sampleNames", name)
		}
	}
}

// TestFixtureLoader_RejectsInvalidEnum verifies fixture validation
// rejects an unknown direction string.
func TestFixtureLoader_RejectsInvalidEnum(t *testing.T) {
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
	_, err := loadFixture(path)
	if err == nil {
		t.Fatalf("loadFixture invalid direction should fail, got nil")
	}
}

// TestFixtureLoader_RejectsMissingProvenance verifies the cc_commit
// field is required.
func TestFixtureLoader_RejectsMissingProvenance(t *testing.T) {
	t.Parallel()
	path := writeFixtureForTest(t, `{
	  "name": "missing-prov",
	  "viewport": {"width": 10, "height": 1},
	  "root": {"label": "root"},
	  "expected": {"root": {"x": 0, "y": 0, "w": 10, "h": 1}}
	}`)
	_, err := loadFixture(path)
	if err == nil {
		t.Fatalf("loadFixture missing cc_commit should fail")
	}
}
