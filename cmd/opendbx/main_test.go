// Copyright 2026 opendbx contributors. See LICENSE.
//
// CLI text golden tests (spec-0.2 D-3). Compares stdout/stderr of `run()`
// against testdata/golden/*.txt. **Not** part of the UI Review pipeline
// (that lives in spec-0.11.5 — PTY golden + freeze + Qwen2.5-VL); this is
// purely a self-contained binary-output regression check.
//
// Update goldens: TEST_UPDATE_GOLDEN=1 go test ./cmd/opendbx -run TestGolden
//
// Author: sqlrush
package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sqlrush/opendbx/internal/platform/version"
)

func TestGolden(t *testing.T) {
	saved := version.Version
	version.Version = "dev"
	defer func() { version.Version = saved }()

	cases := []struct {
		name       string
		args       []string
		stream     string
		exitCode   int
		goldenPath string
	}{
		{"version_long_flag", []string{"--version"}, "stdout", 0, "testdata/golden/version.txt"},
		{"version_short_flag", []string{"-v"}, "stdout", 0, "testdata/golden/version.txt"},
		{"version_subcommand", []string{"version"}, "stdout", 0, "testdata/golden/version.txt"},
		{"help_long_flag", []string{"--help"}, "stdout", 0, "testdata/golden/help.txt"},
		{"help_short_flag", []string{"-h"}, "stdout", 0, "testdata/golden/help.txt"},
		{"no_args_prints_help", []string{}, "stdout", 0, "testdata/golden/help.txt"},
		{"unknown_subcommand_stderr", []string{"foobar"}, "stderr", 1, "testdata/golden/unknown-subcommand.txt"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			got := run(tc.args, &stdout, &stderr)

			if got != tc.exitCode {
				t.Errorf("exit code mismatch: got %d, want %d", got, tc.exitCode)
			}

			var actual string
			switch tc.stream {
			case "stdout":
				actual = stdout.String()
			case "stderr":
				actual = stderr.String()
			default:
				t.Fatalf("invalid stream: %s", tc.stream)
			}

			if os.Getenv("TEST_UPDATE_GOLDEN") == "1" {
				if err := os.MkdirAll(filepath.Dir(tc.goldenPath), 0o755); err != nil {
					t.Fatalf("mkdir golden dir: %v", err)
				}
				if err := os.WriteFile(tc.goldenPath, []byte(actual), 0o644); err != nil {
					t.Fatalf("write golden: %v", err)
				}
				t.Logf("updated golden: %s", tc.goldenPath)
				return
			}

			wantBytes, err := os.ReadFile(tc.goldenPath)
			if err != nil {
				t.Fatalf("read golden %s: %v (run TEST_UPDATE_GOLDEN=1 to create)", tc.goldenPath, err)
			}
			want := string(wantBytes)
			if actual != want {
				t.Errorf("golden mismatch\n--- want (%s) ---\n%s\n--- got ---\n%s\n--- first-mismatch-byte=%d",
					tc.goldenPath, want, actual, firstDiff(actual, want))
			}
		})
	}
}

// TestSubcommandStubs verifies each of the 4 subcommand stubs prints the
// stage-0 marker and routes correctly.
func TestSubcommandStubs(t *testing.T) {
	subs := []string{"interact", "agent", "cluster", "admin"}
	for _, sub := range subs {
		t.Run(sub, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run([]string{sub}, &stdout, &stderr)
			if code != 0 {
				t.Errorf("%s subcommand exit=%d, want 0", sub, code)
			}
			if !strings.Contains(stdout.String(), "not yet implemented in stage 0") {
				t.Errorf("%s stdout missing stage-0 marker. got: %q", sub, stdout.String())
			}
			if !strings.Contains(stdout.String(), sub) {
				t.Errorf("%s stdout missing subcommand name. got: %q", sub, stdout.String())
			}
			if stderr.Len() != 0 {
				t.Errorf("%s should not write to stderr; got: %q", sub, stderr.String())
			}
		})
	}
}

func firstDiff(a, b string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	if len(a) != len(b) {
		return n
	}
	return -1
}
