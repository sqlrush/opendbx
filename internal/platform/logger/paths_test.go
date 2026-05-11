// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package logger

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// setArgvForTesting overrides the package-level argv slice for the duration
// of the calling test. Cleanup restores the prior slice.
func setArgvForTesting(t *testing.T, args ...string) {
	t.Helper()
	argvMu.Lock()
	prev := argvCopy
	argvCopy = append([]string(nil), args...)
	argvMu.Unlock()
	t.Cleanup(func() {
		argvMu.Lock()
		argvCopy = prev
		argvMu.Unlock()
	})
}

func TestIsEnvTruthy(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want bool
	}{
		{"true", true},
		{"TRUE", true},
		{"  True  ", true},
		{"1", true},
		{"yes", true},
		{"YES", true},
		{"false", false},
		{"0", false},
		{"no", false},
		{"", false},
		{"   ", false},
		{"truthy", false}, // not exactly "true"
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			if got := isEnvTruthy(tc.in); got != tc.want {
				t.Errorf("isEnvTruthy(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestGetDebugFilePathEqualsForm(t *testing.T) {
	setArgvForTesting(t, "opendbx", "--debug-file=/tmp/log.txt", "interact")
	got, ok := getDebugFilePath()
	if !ok || got != "/tmp/log.txt" {
		t.Errorf("getDebugFilePath() = (%q, %v), want (/tmp/log.txt, true)", got, ok)
	}
}

// codex MED-2 integration: CC supports `--debug-file <path>` split-arg form.
func TestGetDebugFilePathSplitArgForm(t *testing.T) {
	setArgvForTesting(t, "opendbx", "--debug-file", "/var/log/x.txt", "diag")
	got, ok := getDebugFilePath()
	if !ok || got != "/var/log/x.txt" {
		t.Errorf("getDebugFilePath() split-arg = (%q, %v), want (/var/log/x.txt, true)", got, ok)
	}
}

func TestGetDebugFilePathAbsent(t *testing.T) {
	setArgvForTesting(t, "opendbx", "interact")
	if got, ok := getDebugFilePath(); ok {
		t.Errorf("getDebugFilePath() with no flag = (%q, true), want (\"\", false)", got)
	}
}

// CC parity Q11 1A: `OPENDBX_DEBUG_LOGS_DIR` is treated as a FULL FILE PATH
// even though the name has _DIR suffix. Verbatim from CC behaviour.
func TestGetDebugLogPathEnvFullPath(t *testing.T) {
	setArgvForTesting(t, "opendbx", "interact")
	t.Setenv("OPENDBX_DEBUG_LOGS_DIR", "/custom/path/file.log")
	got := getDebugLogPath("session-abc")
	if got != "/custom/path/file.log" {
		t.Errorf("getDebugLogPath env-override = %q, want /custom/path/file.log (CC verbatim full-path semantics)", got)
	}
}

func TestGetDebugLogPathFlagOverridesEnv(t *testing.T) {
	setArgvForTesting(t, "opendbx", "--debug-file=/flag/path.log")
	t.Setenv("OPENDBX_DEBUG_LOGS_DIR", "/env/path.log")
	got := getDebugLogPath("session-abc")
	if got != "/flag/path.log" {
		t.Errorf("flag should override env: got %q, want /flag/path.log", got)
	}
}

func TestGetDebugLogPathDefault(t *testing.T) {
	setArgvForTesting(t, "opendbx", "interact")
	t.Setenv("OPENDBX_DEBUG_LOGS_DIR", "")
	got := getDebugLogPath("session-abc")
	// Default path embeds the session id under the platform debug dir.
	if !strings.HasSuffix(got, filepath.Join("opendbx", "debug", "session-abc.txt")) {
		t.Errorf("default path = %q, want suffix opendbx/debug/session-abc.txt", got)
	}
}

func TestGetDebugLogPathEmptySessionFallback(t *testing.T) {
	setArgvForTesting(t, "opendbx", "interact")
	t.Setenv("OPENDBX_DEBUG_LOGS_DIR", "")
	got := getDebugLogPath("")
	if !strings.Contains(got, "session.txt") {
		t.Errorf("empty session fallback path = %q, want substring session.txt", got)
	}
}

func TestDebugDirDefault(t *testing.T) {
	dir := debugDirDefault()
	if dir == "" {
		t.Fatal("debugDirDefault() returned empty")
	}
	// Platform-specific structural check.
	switch runtime.GOOS {
	case "windows":
		if !strings.Contains(dir, filepath.Join("opendbx", "debug")) {
			t.Errorf("Windows debug dir = %q, want opendbx/debug suffix", dir)
		}
	default:
		if !strings.Contains(dir, filepath.Join(".opendbx", "debug")) {
			t.Errorf("Unix debug dir = %q, want .opendbx/debug suffix", dir)
		}
	}
}

func TestIsDebugToStdErrLong(t *testing.T) {
	setArgvForTesting(t, "opendbx", "--debug-to-stderr")
	if !isDebugToStdErr() {
		t.Error("isDebugToStdErr() = false for --debug-to-stderr, want true")
	}
}

func TestIsDebugToStdErrShort(t *testing.T) {
	setArgvForTesting(t, "opendbx", "-d2e", "diag")
	if !isDebugToStdErr() {
		t.Error("isDebugToStdErr() = false for -d2e, want true")
	}
}

func TestIsDebugToStdErrAbsent(t *testing.T) {
	setArgvForTesting(t, "opendbx", "interact")
	if isDebugToStdErr() {
		t.Error("isDebugToStdErr() = true with no flag, want false")
	}
}

func TestGetDebugFilterPresent(t *testing.T) {
	setArgvForTesting(t, "opendbx", "--debug=api,hooks")
	f := getDebugFilter()
	if f == nil {
		t.Fatal("getDebugFilter() = nil with --debug=api,hooks")
	}
	if len(f.include) != 2 || f.include[0] != "api" || f.include[1] != "hooks" {
		t.Errorf("filter include = %v, want [api hooks]", f.include)
	}
}

func TestGetDebugFilterAbsent(t *testing.T) {
	setArgvForTesting(t, "opendbx", "interact")
	if f := getDebugFilter(); f != nil {
		t.Errorf("getDebugFilter() = %+v with no --debug=, want nil", f)
	}
}

// CC parity all 7 isDebugMode triggers + the runtime atomic.Bool path.
func TestIsDebugModeTriggers(t *testing.T) {
	cases := []struct {
		name string
		set  func(t *testing.T)
		want bool
	}{
		{
			name: "no signal → false",
			set:  func(t *testing.T) { setArgvForTesting(t, "opendbx", "interact") },
			want: false,
		},
		{
			name: "runtime atomic flip",
			set: func(t *testing.T) {
				setArgvForTesting(t, "opendbx", "interact")
				t.Cleanup(func() { runtimeDebugEnabled.Store(false) })
				EnableDebugLogging()
			},
			want: true,
		},
		{
			name: "ENV DEBUG truthy",
			set: func(t *testing.T) {
				setArgvForTesting(t, "opendbx", "interact")
				t.Setenv("DEBUG", "true")
			},
			want: true,
		},
		{
			name: "ENV DEBUG_SDK truthy",
			set: func(t *testing.T) {
				setArgvForTesting(t, "opendbx", "interact")
				t.Setenv("DEBUG_SDK", "1")
			},
			want: true,
		},
		{
			name: "ENV OPENDBX_DEBUG truthy",
			set: func(t *testing.T) {
				setArgvForTesting(t, "opendbx", "interact")
				t.Setenv("OPENDBX_DEBUG", "yes")
			},
			want: true,
		},
		{
			name: "argv --debug",
			set:  func(t *testing.T) { setArgvForTesting(t, "opendbx", "--debug", "interact") },
			want: true,
		},
		{
			name: "argv -d short form",
			set:  func(t *testing.T) { setArgvForTesting(t, "opendbx", "-d", "interact") },
			want: true,
		},
		{
			name: "argv --debug-to-stderr",
			set:  func(t *testing.T) { setArgvForTesting(t, "opendbx", "--debug-to-stderr") },
			want: true,
		},
		{
			name: "argv -d2e",
			set:  func(t *testing.T) { setArgvForTesting(t, "opendbx", "-d2e") },
			want: true,
		},
		{
			name: "argv --debug=pattern",
			set:  func(t *testing.T) { setArgvForTesting(t, "opendbx", "--debug=api") },
			want: true,
		},
		{
			name: "argv --debug-file= implicit",
			set:  func(t *testing.T) { setArgvForTesting(t, "opendbx", "--debug-file=/tmp/x.log") },
			want: true,
		},
		{
			name: "argv --debug-file <path> implicit",
			set:  func(t *testing.T) { setArgvForTesting(t, "opendbx", "--debug-file", "/tmp/x.log") },
			want: true,
		},
		{
			name: "ENV DEBUG=false NOT truthy",
			set: func(t *testing.T) {
				setArgvForTesting(t, "opendbx", "interact")
				t.Setenv("DEBUG", "false")
			},
			want: false,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// Cannot run in parallel — these tests mutate process-wide ENV +
			// atomic.Bool state.
			runtimeDebugEnabled.Store(false)
			t.Cleanup(func() { runtimeDebugEnabled.Store(false) })
			tc.set(t)
			if got := IsDebugMode(); got != tc.want {
				t.Errorf("IsDebugMode() = %v, want %v", got, tc.want)
			}
		})
	}
}

// claude HIGH-2 contract: EnableDebugLogging is observed by the NEXT
// IsDebugMode call — NOT memoised.
func TestIsDebugModeRuntimeToggleNotMemoised(t *testing.T) {
	setArgvForTesting(t, "opendbx", "interact")
	t.Setenv("DEBUG", "")
	t.Setenv("DEBUG_SDK", "")
	t.Setenv("OPENDBX_DEBUG", "")
	runtimeDebugEnabled.Store(false)
	t.Cleanup(func() { runtimeDebugEnabled.Store(false) })

	if IsDebugMode() {
		t.Fatal("IsDebugMode initially true, want false")
	}
	EnableDebugLogging()
	if !IsDebugMode() {
		t.Fatal("IsDebugMode false after EnableDebugLogging — memoisation regression")
	}
	runtimeDebugEnabled.Store(false)
	if IsDebugMode() {
		t.Fatal("IsDebugMode true after manual runtime flip clear — memoisation regression")
	}
}

func TestGetMinDebugLogLevelDefault(t *testing.T) {
	t.Setenv("OPENDBX_DEBUG_LOG_LEVEL", "")
	if got := getMinDebugLogLevel(); got != LevelDebug {
		t.Errorf("getMinDebugLogLevel default = %v, want LevelDebug", got)
	}
}

func TestGetMinDebugLogLevelEnvOverride(t *testing.T) {
	cases := []struct {
		env  string
		want Level
	}{
		{"verbose", LevelVerbose},
		{"VERBOSE", LevelVerbose},
		{"  Info  ", LevelInfo},
		{"warn", LevelWarn},
		{"error", LevelError},
		{"invalid", LevelDebug}, // unrecognised → fallback
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.env, func(t *testing.T) {
			t.Setenv("OPENDBX_DEBUG_LOG_LEVEL", tc.env)
			if got := getMinDebugLogLevel(); got != tc.want {
				t.Errorf("getMinDebugLogLevel(%q) = %v, want %v", tc.env, got, tc.want)
			}
		})
	}
}
