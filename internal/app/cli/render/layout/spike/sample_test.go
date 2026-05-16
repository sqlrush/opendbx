// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

//go:build spike
// +build spike

package spike

import (
	"path/filepath"
	"testing"
)

// sampleNames lists the 5 locked CC UI sample fixtures (spec-0.12.5 D-2).
// Array type [5]string is intentional (T-10 go-reviewer MED-2): a slice
// `var` is mutable at package scope (slicing / append); a fixed array
// type at exactly 5 is the spec § 2.1 lock made into Go's type system.
// Adding/removing a sample requires editing the array length, which fails
// compilation and forces a spec errata.
var sampleNames = [5]string{
	"repl-prompt",
	"message-block",
	"tool-call-panel",
	"status-line",       // CRITICAL (Row + intrinsic + grow)
	"permission-picker", // CRITICAL (Column + intrinsic + shrink overflow)
}

// TestCCSamples runs the locked 5 CC UI sample fixtures.
// Each fixture has labeled nodes + expected Box positions. Failure mode
// determines outcome per spec-0.12.5 § 2.2 outcome table:
//   - all 5 pass + critical pass + perf < 5ms → A 自研
//   - critical sample fails → C (forced regardless of普通 sample count)
//   - ≤ 3/5 普通 sample pass → B yoga-go
func TestCCSamples(t *testing.T) {
	t.Parallel()
	pass, fail, criticalFail := runAllSamples(t)
	t.Logf("CC samples: %d pass / %d fail (critical fail: %d)", pass, fail, criticalFail)
	if criticalFail > 0 {
		t.Errorf("critical sample failure: %d → outcome C/B (spec § 2.2)", criticalFail)
	}
	if pass < 4 {
		t.Errorf("only %d/5 sample pass (< 4) → outcome B (spec § 2.2)", pass)
	}
}

// runAllSamples returns (pass, fail, criticalFail) counts.
func runAllSamples(t *testing.T) (pass, fail, criticalFail int) {
	t.Helper()
	for _, name := range sampleNames {
		fx, err := LoadFixture(filepath.Join("testdata", "cc-samples", name+".json"))
		if err != nil {
			t.Errorf("load fixture %s: %v", name, err)
			fail++
			continue
		}
		root, index, err := fx.Root.BuildTree()
		if err != nil {
			t.Errorf("build tree %s: %v", name, err)
			fail++
			continue
		}
		got := Layout(root, fx.Viewport.ViewportBox())
		ok := true
		for label, expected := range fx.Expected {
			node, exists := index[label]
			if !exists {
				t.Errorf("[%s] expected label %q not in tree", name, label)
				ok = false
				continue
			}
			if got[node] != expected.ToBox() {
				t.Errorf("[%s] %s: got %+v, want %+v", name, label, got[node], expected.ToBox())
				ok = false
			}
		}
		if ok {
			pass++
		} else {
			fail++
			if fx.Critical {
				criticalFail++
			}
		}
	}
	return pass, fail, criticalFail
}

// TestSampleLockedAt5 asserts the spec-0.12.5 § 2.1 / § 11.1 "5 locked" rule.
// Trivially true because sampleNames is typed [5]string; adding / removing
// requires editing the type. T-10 go-reviewer MED-2 hardening.
func TestSampleLockedAt5(t *testing.T) {
	t.Parallel()
	if len(sampleNames) != 5 {
		t.Errorf("sample count = %d, want 5 (locked per spec-0.12.5 § 2.1)", len(sampleNames))
	}
}

// TestCriticalSamplesPresent asserts the two critical samples (status-line
// + permission-picker) are present and marked critical in their fixture
// JSON — spec § 1.3 D-2 critical sample gate.
func TestCriticalSamplesPresent(t *testing.T) {
	t.Parallel()
	inSet := map[string]bool{"status-line": false, "permission-picker": false}
	for _, name := range sampleNames {
		fx, err := LoadFixture(filepath.Join("testdata", "cc-samples", name+".json"))
		if err != nil {
			t.Fatalf("load fixture %s: %v", name, err)
		}
		if _, required := inSet[name]; required {
			if !fx.Critical {
				t.Errorf("%s: critical=false in fixture, but spec § 1.3 D-2 says critical", name)
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
