// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

//go:build !windows

package visualgolden

import (
	"bytes"
	"errors"
	"fmt"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeFreezeScript writes a stub shell script to tmpDir that accepts
// `freeze -o <path> ...` invocation, ignores stdin, and writes a fixed
// PNG to <path>. Returns the script path for FREEZE_BIN env.
func fakeFreezeScript(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	// Pre-generate a small valid PNG.
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, fillImage(10, 10, color.RGBA{R: 0, G: 50, B: 200, A: 255})); err != nil {
		t.Fatalf("encode stub png: %v", err)
	}
	pngPath := filepath.Join(tmpDir, "stub.png")
	if err := os.WriteFile(pngPath, pngBuf.Bytes(), 0o600); err != nil {
		t.Fatalf("write stub png: %v", err)
	}

	script := filepath.Join(tmpDir, "freeze-fake.sh")
	body := fmt.Sprintf(`#!/bin/sh
# Parse args: look for -o <path>; copy stub PNG there.
while [ $# -gt 0 ]; do
  case "$1" in
    -o) shift; cp '%s' "$1"; shift ;;
    *)  shift ;;
  esac
done
exit 0
`, pngPath)
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatalf("write stub script: %v", err)
	}
	return script
}

func TestRender_FakeBinary(t *testing.T) {
	// NOT t.Parallel — sets FREEZE_BIN env.
	t.Setenv("FREEZE_BIN", fakeFreezeScript(t))
	got := Render(t, []byte("\x1b[31mhello\x1b[0m"), DefaultTheme())
	if len(got) == 0 {
		t.Error("Render returned empty bytes")
	}
	if _, err := png.Decode(bytes.NewReader(got)); err != nil {
		t.Errorf("Render output should be valid PNG: %v", err)
	}
}

func TestRender_MissingBinary_DevSkip(t *testing.T) {
	// NOT t.Parallel — sets FREEZE_BIN env to nonexistent path.
	t.Setenv("FREEZE_BIN", "/definitely/nonexistent/freeze")
	// VISUALGOLDEN_REQUIRED unset → Skipf. Use a mockT to capture.
	t.Setenv("VISUALGOLDEN_REQUIRED", "")
	mt := &skipMockT{}
	Render(mt, []byte("anything"), DefaultTheme())
	if !mt.skipCalled {
		t.Error("expected Skipf when freeze missing in dev mode")
	}
}

func TestRender_MissingBinary_CIFatal(t *testing.T) {
	// NOT t.Parallel — sets env.
	t.Setenv("FREEZE_BIN", "/definitely/nonexistent/freeze")
	t.Setenv("VISUALGOLDEN_REQUIRED", "1")
	mt := &skipMockT{}
	Render(mt, []byte("anything"), DefaultTheme())
	if !mt.fatalCalled {
		t.Error("expected Fatalf when freeze missing + VISUALGOLDEN_REQUIRED set")
	}
	if !strings.Contains(mt.fatalMsg, "VISUALGOLDEN_REQUIRED") {
		t.Errorf("fatal msg should mention env var; got %q", mt.fatalMsg)
	}
}

func TestFreezeBin_InvalidEnvPaths(t *testing.T) {
	// NOT t.Parallel — sets FREEZE_BIN env.
	dir := t.TempDir()
	t.Setenv("FREEZE_BIN", dir)
	if _, err := freezeBin(); !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("directory FREEZE_BIN should return os.ErrInvalid; got %v", err)
	}

	file := filepath.Join(dir, "not-executable")
	if err := os.WriteFile(file, []byte("#!/bin/sh\n"), 0o600); err != nil {
		t.Fatalf("write non-executable: %v", err)
	}
	t.Setenv("FREEZE_BIN", file)
	if _, err := freezeBin(); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("non-executable FREEZE_BIN should return os.ErrPermission; got %v", err)
	}
}

func TestRender_CommandFailureFatal(t *testing.T) {
	// NOT t.Parallel — sets FREEZE_BIN env.
	dir := t.TempDir()
	script := filepath.Join(dir, "freeze-fail.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho boom >&2\nexit 7\n"), 0o700); err != nil {
		t.Fatalf("write fail script: %v", err)
	}
	t.Setenv("FREEZE_BIN", script)
	mt := &skipMockT{}
	Render(mt, []byte("anything"), DefaultTheme())
	if !mt.fatalCalled || !strings.Contains(mt.fatalMsg, "boom") {
		t.Fatalf("expected freeze stderr in fatal; got %q", mt.fatalMsg)
	}
}

func TestRender_InvalidPNGOutputFatal(t *testing.T) {
	// NOT t.Parallel — sets FREEZE_BIN env.
	dir := t.TempDir()
	script := filepath.Join(dir, "freeze-text.sh")
	body := `#!/bin/sh
while [ $# -gt 0 ]; do
  case "$1" in
    -o) shift; printf 'not a png' > "$1"; shift ;;
    *)  shift ;;
  esac
done
exit 0
`
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatalf("write text script: %v", err)
	}
	t.Setenv("FREEZE_BIN", script)
	mt := &skipMockT{}
	Render(mt, []byte("anything"), DefaultTheme())
	if !mt.fatalCalled || !strings.Contains(mt.fatalMsg, "not valid PNG") {
		t.Fatalf("expected invalid PNG fatal; got %q", mt.fatalMsg)
	}
}

// skipMockT extends mockT with Skipf tracking.
type skipMockT struct {
	mockT
	skipCalled bool
	skipMsg    string
}

func (s *skipMockT) Skipf(format string, args ...any) {
	s.skipCalled = true
	s.skipMsg = fmt.Sprintf(format, args...)
}

func (s *skipMockT) TempDir() string {
	dir, err := os.MkdirTemp("", "render-mock-")
	if err != nil {
		panic("mockT.TempDir: " + err.Error())
	}
	return dir
}
