// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

//go:build !windows

package visualgolden

import (
	"bytes"
	"flag"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// updateVisualOracle is the test seam parallel to golden.updateOracle.
// Production reads -update-visual flag via Update(); tests swap.
//
//nolint:gochecknoglobals // spec-0.11.5 D-3: flag registration side effect + test seam.
var updateVisualOracle = Update

// init registers -update-visual once per binary unless already
// registered (mirrors golden package pattern; spec-0.11.5 D-2).
//
//nolint:gochecknoinits // spec-0.11.5 D-3: flag registration is intentional side effect.
func init() {
	if flag.Lookup("update-visual") != nil {
		return
	}
	flag.Bool("update-visual", false, "update visual golden PNGs on mismatch")
}

// Update reports whether -update-visual is set.
func Update() bool {
	f := flag.Lookup("update-visual")
	if f == nil {
		return false
	}
	g, ok := f.Value.(flag.Getter)
	if !ok {
		return false
	}
	b, ok := g.Get().(bool)
	if !ok {
		return false
	}
	return b
}

// Compare diffs got PNG against testdata/visual/<TestName>[/<sub>].png
// using maxMismatchFraction as max fraction of differing pixels.
//
// On -update-visual flag, writes got. On dimension mismatch or read
// error, fails before pixel diff.
//
// maxMismatchFraction (0..1): typically 0.01 (1%). Distinct from
// Diff's pixelSensitivity (R5 spec § 2.4 to avoid reversed-semantic
// confusion).
//
// Metadata sidecar drift check (spec § 2.3 R3 codex HIGH-3) deferred
// to T-13 errata: captureMetadata implementation requires reliable
// freeze/rsvg version probes which are non-trivial for CI portability.
func Compare(t testing.TB, name string, got []byte, maxMismatchFraction float64) {
	t.Helper()
	path := goldenPath(t, name)
	compareAt(t, path, got, maxMismatchFraction)
}

func compareAt(t testing.TB, path string, got []byte, maxMismatchFraction float64) {
	t.Helper()
	if updateVisualOracle() {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("visualgolden: mkdir %s: %v", filepath.Dir(path), err)
			return
		}
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatalf("visualgolden: write %s: %v", path, err)
			return
		}
		return
	}
	want, err := os.ReadFile(path) // #nosec G304 -- spec-0.11.5 D-2: caller-controlled fixture path
	if err != nil {
		t.Fatalf("visualgolden: read %s: %v (run with -update-visual to create)", path, err)
		return
	}
	wantImg, err := png.Decode(bytes.NewReader(want))
	if err != nil {
		t.Fatalf("visualgolden: decode golden %s: %v", path, err)
		return
	}
	gotImg, err := png.Decode(bytes.NewReader(got))
	if err != nil {
		t.Fatalf("visualgolden: decode got: %v", err)
		return
	}
	if wantImg.Bounds() != gotImg.Bounds() {
		t.Errorf("visualgolden: dimension mismatch for %s: want %v, got %v",
			path, wantImg.Bounds(), gotImg.Bounds())
		return
	}
	// pixelSensitivity 0.1 is moderate (mapbox default ≈ 0.1).
	mismatched, _ := Diff(wantImg, gotImg, 0.1)
	total := wantImg.Bounds().Dx() * wantImg.Bounds().Dy()
	fraction := float64(mismatched) / float64(total)
	if fraction > maxMismatchFraction {
		t.Errorf("visualgolden: %s mismatch %d/%d pixels (%.2f%% > %.2f%% threshold)",
			path, mismatched, total, fraction*100, maxMismatchFraction*100)
	}
}

// goldenPath returns testdata/visual/<TestName>[/<sub>].png relative
// to the caller test file's dir.
func goldenPath(t testing.TB, name string) string {
	t.Helper()
	testName := t.Name()
	rel := filepath.Join("testdata", "visual", testName)
	if name != "" {
		rel = filepath.Join(rel, name)
	}
	return filepath.Join(callerDir(t), rel+".png")
}

// callerDir walks the stack skipping our own non-test frames.
func callerDir(t testing.TB) string {
	t.Helper()
	for skip := 2; skip < 16; skip++ {
		_, file, _, ok := runtime.Caller(skip)
		if !ok {
			break
		}
		if strings.Contains(file, "/internal/testing/visualgolden/") && !strings.HasSuffix(file, "_test.go") {
			continue
		}
		return filepath.Dir(file)
	}
	dir, _ := os.Getwd()
	return dir
}
