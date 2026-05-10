// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// CLI golden tests (spec-0.3 D-6).
//
// Replaces spec-0.2 D-3's flag-stdlib-based golden tests. Now uses cobra's
// SetArgs / SetOut / SetErr to exercise the same root command as the real
// binary. Tests are intentionally not exec-the-binary (which would slow CI
// 50x); instead we drive newRootCommand() in-process.
//
// Update goldens: TEST_UPDATE_GOLDEN=1 go test ./cmd/opendbx -run TestGolden

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sqlrush/opendbx/internal/platform/version"
)

// runCmd executes the root command with the given args and returns stdout,
// stderr, and exit error. Uses a fresh root command per invocation so cobra
// state cannot leak.
func runCmd(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	cmd := newRootCommand()
	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(args)
	err = cmd.Execute()
	return outBuf.String(), errBuf.String(), err
}

func TestGolden(t *testing.T) {
	saved := version.Version
	version.Version = "dev"
	defer func() { version.Version = saved }()

	cases := []struct {
		name       string
		args       []string
		stream     string // "stdout" or "stderr"
		expectErr  bool
		goldenPath string
	}{
		// version
		{"version_long_flag", []string{"--version"}, "stdout", false, "testdata/golden/version.txt"},
		{"version_short_flag", []string{"-v"}, "stdout", false, "testdata/golden/version.txt"},
		{"version_subcommand", []string{"version"}, "stdout", false, "testdata/golden/version.txt"},

		// help (root)
		{"help_long_flag", []string{"--help"}, "stdout", false, "testdata/golden/help.txt"},
		{"help_short_flag", []string{"-h"}, "stdout", false, "testdata/golden/help.txt"},

		// no-args → interact stub (D1 decision)
		{"no_args_interact_stub", []string{}, "stdout", false, "testdata/golden/no-args-interact-stub.txt"},

		// "non-subcommand bare arg" — CC parity: `opendbx foobar` treats foobar as
		// [prompt], NOT as an unknown subcommand error. (`claude foobar` does the
		// same — it sends "foobar" to the LLM.) Verified empirically against CC
		// v2.1.138.
		{"bare_arg_as_prompt", []string{"foobar"}, "stdout", false, "testdata/golden/bare-arg-as-prompt.txt"},

		// unknown flag — cobra correctly returns error
		{"unknown_flag_stderr", []string{"--xyz-not-a-real-flag"}, "stderr", true, "testdata/golden/unknown-flag.txt"},

		// subcommand --help (one per visible subcommand)
		{"interact_help", []string{"interact", "--help"}, "stdout", false, "testdata/golden/interact-help.txt"},
		{"agent_help", []string{"agent", "--help"}, "stdout", false, "testdata/golden/agent-help.txt"},
		{"cluster_help", []string{"cluster", "--help"}, "stdout", false, "testdata/golden/cluster-help.txt"},
		{"admin_help", []string{"admin", "--help"}, "stdout", false, "testdata/golden/admin-help.txt"},
		{"mcp_help", []string{"mcp", "--help"}, "stdout", false, "testdata/golden/mcp-help.txt"},
		{"plugin_help", []string{"plugin", "--help"}, "stdout", false, "testdata/golden/plugin-help.txt"},
		{"auth_help", []string{"auth", "--help"}, "stdout", false, "testdata/golden/auth-help.txt"},
		{"agents_help", []string{"agents", "--help"}, "stdout", false, "testdata/golden/agents-help.txt"},
		{"doctor_help", []string{"doctor", "--help"}, "stdout", false, "testdata/golden/doctor-help.txt"},
		{"update_help", []string{"update", "--help"}, "stdout", false, "testdata/golden/update-help.txt"},
		{"install_help", []string{"install", "--help"}, "stdout", false, "testdata/golden/install-help.txt"},
		{"completion_help", []string{"completion", "--help"}, "stdout", false, "testdata/golden/completion-help.txt"},
		{"open_help", []string{"open", "--help"}, "stdout", false, "testdata/golden/open-help.txt"},
		{"db_help", []string{"db", "--help"}, "stdout", false, "testdata/golden/db-help.txt"},
		{"sentinel_help", []string{"sentinel", "--help"}, "stdout", false, "testdata/golden/sentinel-help.txt"},
		{"diag_help", []string{"diag", "--help"}, "stdout", false, "testdata/golden/diag-help.txt"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, err := runCmd(t, tc.args...)
			if tc.expectErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tc.expectErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			var actual string
			switch tc.stream {
			case "stdout":
				actual = stdout
			case "stderr":
				// cobra writes "Error: ...\nUsage: ...\n" to the err writer for unknown
				// subcommands; both streams may contain content.
				actual = stderr
				if actual == "" {
					actual = stdout
				}
			default:
				t.Fatalf("invalid stream: %s", tc.stream)
			}

			if os.Getenv("TEST_UPDATE_GOLDEN") == "1" {
				if err := os.MkdirAll(filepath.Dir(tc.goldenPath), 0o750); err != nil {
					t.Fatalf("mkdir golden dir: %v", err)
				}
				if err := os.WriteFile(tc.goldenPath, []byte(actual), 0o600); err != nil {
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

func TestSubcommandStubs(t *testing.T) {
	subs := []struct {
		args []string
		spec string
	}{
		{[]string{"agent"}, "spec-9.X"},
		{[]string{"cluster"}, "spec-9.X"},
		{[]string{"admin", "migrate"}, "spec-4.8-version-migrations"},
		{[]string{"mcp", "list"}, "spec-2.5"},
		{[]string{"plugin", "list"}, "spec-2.1-skill-md-format"},
		{[]string{"auth", "status"}, "Stage 2+"},
		{[]string{"doctor"}, "Stage 4+"},
		{[]string{"update"}, "spec-4.7-install"},
		{[]string{"install"}, "spec-4.7-install"},
		{[]string{"setup-token"}, "Stage 2+"},
		{[]string{"db", "list"}, "spec-1.19-connection-config"},
		{[]string{"sentinel", "status"}, "Stage 1+"},
		{[]string{"diag", "slow-sql"}, "spec-1.21-diagnose-loop"},
	}
	for _, tc := range subs {
		name := strings.Join(tc.args, "_")
		t.Run(name, func(t *testing.T) {
			stdout, _, err := runCmd(t, tc.args...)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !strings.Contains(stdout, "not yet implemented") {
				t.Errorf("%s missing 'not yet implemented' marker. got: %q", name, stdout)
			}
			if !strings.Contains(stdout, tc.spec) {
				t.Errorf("%s missing target spec %q. got: %q", name, tc.spec, stdout)
			}
		})
	}
}

func TestRoot_PromptArgument(t *testing.T) {
	stdout, _, err := runCmd(t, "hello world")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "received prompt: \"hello world\"") {
		t.Errorf("[prompt] positional not captured. stdout: %q", stdout)
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
