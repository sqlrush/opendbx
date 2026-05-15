// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

//go:build !windows

package visualgolden

import (
	"bytes"
	"fmt"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// freezeBin resolves the freeze binary path. Production reads
// LookPath; tests can swap to a stub binary by setting
// FREEZE_BIN env var. Validates the path is an executable file.
//
// spec-0.11.5 T-13 errata (security MED-2): verify exec bit so a
// config file mistakenly placed there produces a clear error rather
// than a confusing "permission denied" surfaced from exec.Command.
func freezeBin() (string, error) {
	if env := os.Getenv("FREEZE_BIN"); env != "" {
		info, err := os.Stat(env)
		if err != nil {
			return "", err
		}
		if info.IsDir() {
			return "", os.ErrInvalid
		}
		if info.Mode()&0o111 == 0 {
			return "", fmt.Errorf("%w: FREEZE_BIN=%s is not executable", os.ErrPermission, env)
		}
		return env, nil
	}
	return exec.LookPath("freeze")
}

// Render invokes freeze to convert ANSI bytes into a PNG. Uses
// freeze -o <tmp>/out.png (-o is output PATH not format selector;
// spec § 2.3 R3 codex CRIT-1 fix).
//
// Behavior on missing freeze binary:
//   - VISUALGOLDEN_REQUIRED=1 → t.Fatalf (CI per spec D-5 ui-visual)
//   - otherwise               → t.Skipf (dev local convenience)
func Render(t testing.TB, ansi []byte, theme Theme) []byte {
	t.Helper()
	bin, err := freezeBin()
	if err != nil {
		if os.Getenv("VISUALGOLDEN_REQUIRED") != "" {
			t.Fatalf("freeze not in PATH and VISUALGOLDEN_REQUIRED set: %v", err)
			return nil
		}
		t.Skipf("freeze not in PATH (dev): %v (install: GOTOOLCHAIN=auto go install github.com/charmbracelet/freeze@v0.2.2)", err)
		return nil
	}
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "out.png")
	cmd := exec.Command(bin, // #nosec G204 -- spec-0.11.5 D-2: caller-controlled freeze binary path
		"-o", outPath,
		"--font.family", theme.FontFamily,
		"--font.size", fmt.Sprint(theme.FontSize),
		"--background", theme.Background,
		"--padding", fmt.Sprint(theme.Padding),
	)
	cmd.Stdin = bytes.NewReader(ansi)
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		t.Fatalf("freeze: %v (stderr: %s)", err, errBuf.String())
		return nil
	}
	got, err := os.ReadFile(outPath) // #nosec G304 -- spec-0.11.5 D-2: tmpDir is t.TempDir owned
	if err != nil {
		t.Fatalf("read freeze output: %v", err)
		return nil
	}
	// R5 codex CRIT-1 smoke: assert bytes are real PNG, not the
	// "WROTE png" ASCII status that R1 spec mistakenly captured.
	if _, err := png.Decode(bytes.NewReader(got)); err != nil {
		t.Fatalf("freeze output is not valid PNG: %v (stderr: %s)", err, errBuf.String())
		return nil
	}
	return got
}
