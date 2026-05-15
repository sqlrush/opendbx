// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

//go:build !windows

package visualgolden

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func fillImage(w, h int, c color.Color) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

// --- Diff -----------------------------------------------------------

func TestDiff_Identical(t *testing.T) {
	t.Parallel()
	a := fillImage(20, 20, color.RGBA{R: 100, G: 100, B: 100, A: 255})
	b := fillImage(20, 20, color.RGBA{R: 100, G: 100, B: 100, A: 255})
	mismatched, diff := Diff(a, b, 0.1)
	if mismatched != 0 {
		t.Errorf("identical images should report 0 mismatched; got %d", mismatched)
	}
	if diff == nil {
		t.Error("diff image should be non-nil even for identical")
	}
}

func TestDiff_AllDifferent(t *testing.T) {
	t.Parallel()
	a := fillImage(10, 10, color.RGBA{R: 0, G: 0, B: 0, A: 255})
	b := fillImage(10, 10, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	mismatched, diff := Diff(a, b, 0.1)
	if mismatched != 100 {
		t.Errorf("black vs white should report all 100 pixels; got %d", mismatched)
	}
	if diff == nil {
		t.Error("diff image should be non-nil")
	}
}

func TestDiff_DimensionMismatch(t *testing.T) {
	t.Parallel()
	a := fillImage(10, 10, color.RGBA{R: 0, G: 0, B: 0, A: 255})
	b := fillImage(20, 20, color.RGBA{R: 0, G: 0, B: 0, A: 255})
	mismatched, diff := Diff(a, b, 0.1)
	if mismatched != -1 || diff != nil {
		t.Errorf("dimension mismatch should return (-1, nil); got (%d, %v)", mismatched, diff)
	}
}

func TestDiff_SinglePixel(t *testing.T) {
	t.Parallel()
	a := fillImage(10, 10, color.RGBA{R: 100, G: 100, B: 100, A: 255})
	b := fillImage(10, 10, color.RGBA{R: 100, G: 100, B: 100, A: 255})
	// Large RGB delta to clearly exceed YIQ-distance × 0.1 threshold.
	b.Set(5, 5, color.RGBA{R: 250, G: 0, B: 0, A: 255})
	mismatched, _ := Diff(a, b, 0.1)
	if mismatched != 1 {
		t.Errorf("single-pixel change should report 1 mismatched; got %d", mismatched)
	}
}

// --- Update flag --------------------------------------------------

func TestUpdate_DefaultFalse(t *testing.T) {
	t.Parallel()
	if Update() {
		t.Error("default -update-visual should be false")
	}
}

// --- Compare public entry (covers goldenPath + callerDir) ---------

func TestCompare_PublicEntry(t *testing.T) {
	// NOT t.Parallel — mutates updateVisualOracle.
	prev := updateVisualOracle
	updateVisualOracle = func() bool { return true }
	t.Cleanup(func() {
		updateVisualOracle = prev
		_ = os.RemoveAll(filepath.Join("testdata", "visual", "TestCompare_PublicEntry"))
	})
	pixels := encodePNG(t, 10, 10, color.RGBA{R: 0, G: 100, B: 0, A: 255})
	Compare(t, "snap", pixels, 0.01)
	// Verify the golden was written under testdata/visual/<TestName>/snap.png.
	expected := filepath.Join("testdata", "visual", "TestCompare_PublicEntry", "snap.png")
	if _, err := os.Stat(expected); err != nil {
		t.Errorf("expected golden at %s: %v", expected, err)
	}
}

// --- compareAt happy + missing/mismatch paths --------------------

func encodePNG(t *testing.T, w, h int, c color.Color) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, fillImage(w, h, c)); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func TestCompareAt_Match(t *testing.T) {
	// NOT t.Parallel — mutates updateVisualOracle global.
	prev := updateVisualOracle
	updateVisualOracle = func() bool { return false }
	t.Cleanup(func() { updateVisualOracle = prev })

	dir := t.TempDir()
	path := filepath.Join(dir, "snap.png")
	pixels := encodePNG(t, 20, 20, color.RGBA{R: 0, G: 200, B: 0, A: 255})
	if err := os.WriteFile(path, pixels, 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	mt := &mockT{}
	compareAt(mt, path, pixels, 0.01)
	if mt.fatalCalled || mt.errorfCalled {
		t.Errorf("expected match; fatal=%v errorf=%v", mt.fatalMsg, mt.errorfMsg)
	}
}

func TestCompareAt_Mismatch(t *testing.T) {
	// NOT t.Parallel — mutates updateVisualOracle global.
	prev := updateVisualOracle
	updateVisualOracle = func() bool { return false }
	t.Cleanup(func() { updateVisualOracle = prev })

	dir := t.TempDir()
	path := filepath.Join(dir, "snap.png")
	wantPixels := encodePNG(t, 20, 20, color.RGBA{R: 0, G: 0, B: 0, A: 255})
	gotPixels := encodePNG(t, 20, 20, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	if err := os.WriteFile(path, wantPixels, 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	mt := &mockT{}
	compareAt(mt, path, gotPixels, 0.01)
	if !mt.errorfCalled {
		t.Error("expected errorf on mismatch")
	}
}

func TestCompareAt_MissingGolden(t *testing.T) {
	// NOT t.Parallel — mutates updateVisualOracle global.
	prev := updateVisualOracle
	updateVisualOracle = func() bool { return false }
	t.Cleanup(func() { updateVisualOracle = prev })

	mt := &mockT{}
	compareAt(mt, "/definitely/nonexistent/path/x.png", []byte{}, 0.01)
	if !mt.fatalCalled {
		t.Error("expected fatal for missing golden")
	}
}

func TestCompareAt_UpdateWritesGolden(t *testing.T) {
	// NOT t.Parallel — mutates updateVisualOracle global.
	prev := updateVisualOracle
	updateVisualOracle = func() bool { return true }
	t.Cleanup(func() { updateVisualOracle = prev })

	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "deep", "snap.png")
	pixels := encodePNG(t, 10, 10, color.RGBA{R: 50, G: 50, B: 50, A: 255})

	mt := &mockT{}
	compareAt(mt, path, pixels, 0.01)
	if mt.fatalCalled {
		t.Fatalf("update should not fail: %s", mt.fatalMsg)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("golden not written: %v", err)
	}
}

func TestCompareAt_DimensionMismatch(t *testing.T) {
	// NOT t.Parallel — mutates updateVisualOracle global.
	prev := updateVisualOracle
	updateVisualOracle = func() bool { return false }
	t.Cleanup(func() { updateVisualOracle = prev })

	dir := t.TempDir()
	path := filepath.Join(dir, "snap.png")
	wantPixels := encodePNG(t, 20, 20, color.RGBA{R: 0, G: 0, B: 0, A: 255})
	gotPixels := encodePNG(t, 30, 30, color.RGBA{R: 0, G: 0, B: 0, A: 255})
	if err := os.WriteFile(path, wantPixels, 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	mt := &mockT{}
	compareAt(mt, path, gotPixels, 0.01)
	if !mt.errorfCalled {
		t.Error("expected errorf on dimension mismatch")
	}
}

// --- mockT ---------------------------------------------------------

type mockT struct {
	testing.TB
	fatalCalled  bool
	fatalMsg     string
	errorfCalled bool
	errorfMsg    string
}

func (m *mockT) Helper() {}

func (m *mockT) Name() string { return "MockTest" }

func (m *mockT) Fatalf(format string, args ...any) {
	m.fatalCalled = true
	m.fatalMsg = fmt.Sprintf(format, args...)
}

func (m *mockT) Errorf(format string, args ...any) {
	m.errorfCalled = true
	m.errorfMsg = fmt.Sprintf(format, args...)
}
