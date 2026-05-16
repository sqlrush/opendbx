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
// Order must not change; sample set is locked per spec § 2.1.
var sampleNames = []string{
	"repl-prompt",
	"message-block",
	"tool-call-panel",
	"status-line",       // CRITICAL (Row + intrinsic + grow)
	"permission-picker", // CRITICAL (Column + intrinsic + shrink)
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
		root, index := fx.Root.BuildTree()
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
// If sample count drifts (added or removed), this test must change in
// lockstep with a spec errata.
func TestSampleLockedAt5(t *testing.T) {
	t.Parallel()
	if len(sampleNames) != 5 {
		t.Errorf("sample count = %d, want 5 (locked per spec-0.12.5 § 2.1)", len(sampleNames))
	}
}

func TestCriticalSamplesPresent(t *testing.T) {
	t.Parallel()
	mustCritical := map[string]bool{"status-line": false, "permission-picker": false}
	for _, name := range sampleNames {
		fx, err := LoadFixture(filepath.Join("testdata", "cc-samples", name+".json"))
		if err != nil {
			t.Fatalf("load fixture %s: %v", name, err)
		}
		if _, want := mustCritical[name]; want {
			if !fx.Critical {
				t.Errorf("%s: critical=false in fixture, but spec § 1.3 D-2 says critical", name)
			}
			mustCritical[name] = true
		}
	}
	for name, found := range mustCritical {
		if !found {
			t.Errorf("required critical sample %q missing from sampleNames", name)
		}
	}
}
