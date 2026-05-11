// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package logger

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// Process-level argv source. Tests substitute this to drive deterministic
// flag parsing without restarting. Defaults to os.Args.
var (
	argvMu   sync.RWMutex
	argvCopy []string
)

// processArgv returns the current view of process arguments. Initialised
// lazily to os.Args on first read; can be overridden via setArgvForTesting.
func processArgv() []string {
	argvMu.RLock()
	if argvCopy != nil {
		v := argvCopy
		argvMu.RUnlock()
		return v
	}
	argvMu.RUnlock()
	argvMu.Lock()
	defer argvMu.Unlock()
	if argvCopy == nil {
		// Copy so tests can mutate process state without affecting us.
		argvCopy = append([]string(nil), os.Args...)
	}
	return argvCopy
}

// getDebugFilePath returns the path passed via `--debug-file=<path>` or
// `--debug-file <path>`. Empty string + false if no such flag is present.
//
// CC parity (debug.ts:91-103): supports BOTH the equals form AND the
// space-separated form. codex MED-2 integration.
//
// Not memoised — argv could theoretically be swapped via setArgvForTesting
// between calls (typical CC behaviour is process.argv frozen at startup; we
// keep the lookup cheap so the live-read is acceptable).
func getDebugFilePath() (string, bool) {
	argv := processArgv()
	for i, arg := range argv {
		if strings.HasPrefix(arg, "--debug-file=") {
			return strings.TrimPrefix(arg, "--debug-file="), true
		}
		if arg == "--debug-file" && i+1 < len(argv) {
			return argv[i+1], true
		}
	}
	return "", false
}

// getDebugLogPath resolves the active debug log file path using the CC 1:1
// priority chain (debug.ts:231-235):
//
//  1. --debug-file=<path> / --debug-file <path>   (highest)
//  2. OPENDBX_DEBUG_LOGS_DIR env var               (treated as FULL path; CC
//     parity per Q11 1A — name has _DIR suffix but behaviour is full path)
//  3. <configHome>/debug/<sessionId>.txt           (default)
//
// sessionID parameter is plumbed in by Init (T-3) so tests can pin a
// deterministic id. Empty sessionID falls back to "session" placeholder
// (the impl supplies a UUID v4 in T-3 — T-8 hardens it to RFC 4122).
func getDebugLogPath(sessionID string) string {
	if p, ok := getDebugFilePath(); ok {
		return p
	}
	if env := os.Getenv("OPENDBX_DEBUG_LOGS_DIR"); env != "" {
		// CC verbatim: env var name has _DIR but value is a full file path.
		return env
	}
	if sessionID == "" {
		sessionID = "session"
	}
	return filepath.Join(debugDirDefault(), sessionID+".txt")
}

// debugDirDefault returns the default `<configHome>/debug` directory on the
// current platform. Mirrors `getClaudeConfigHomeDir()` for opendbx —
// canonical location is `~/.opendbx/debug/`.
//
// Cross-platform handling:
//   - macOS / Linux: $HOME/.opendbx/debug (opendb 老版 lineage; spec § 1.3)
//   - Windows: %APPDATA%/opendbx/debug
//
// Falls back to /tmp/opendbx/debug if HOME cannot be resolved (avoid panic).
func debugDirDefault() string {
	switch runtime.GOOS {
	case "windows":
		appdata := os.Getenv("APPDATA")
		if appdata == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return filepath.Join(os.TempDir(), "opendbx", "debug")
			}
			appdata = filepath.Join(home, "AppData", "Roaming")
		}
		return filepath.Join(appdata, "opendbx", "debug")
	default:
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(os.TempDir(), "opendbx", "debug")
		}
		return filepath.Join(home, ".opendbx", "debug")
	}
}

// isDebugToStdErr reports whether `--debug-to-stderr` or `-d2e` is present
// in argv. CC parity (debug.ts:85-89).
func isDebugToStdErr() bool {
	argv := processArgv()
	for _, arg := range argv {
		if arg == "--debug-to-stderr" || arg == "-d2e" {
			return true
		}
	}
	return false
}

// getDebugFilter returns the parsed `--debug=<pattern>` filter, or nil if
// no such flag is present or the pattern is empty / blank / mixed-mode.
//
// CC parity (debug.ts:73-83): looks for the first argv entry of the form
// `--debug=<pattern>`; pattern parsing rules live in filter.go (D-3).
func getDebugFilter() *debugFilter {
	argv := processArgv()
	for _, arg := range argv {
		if strings.HasPrefix(arg, "--debug=") {
			return parseDebugFilter(strings.TrimPrefix(arg, "--debug="))
		}
	}
	return nil
}

// IsDebugMode reports whether debug logging is enabled for this process.
//
// Returns true if ANY of these conditions hold (CC parity, debug.ts:44-57):
//
//  1. runtimeDebugEnabled atomic.Bool set (EnableDebugLogging called)
//  2. ENV DEBUG / DEBUG_SDK truthy (CC heritage; shared name)
//  3. ENV OPENDBX_DEBUG truthy (opendbx-only extension, § 2.5)
//  4. argv contains "--debug" or "-d"
//  5. argv contains "--debug-to-stderr" or "-d2e"
//  6. argv contains "--debug=<pattern>"
//  7. argv contains "--debug-file=<path>" or "--debug-file <path>" (implicit)
//
// **Not memoised**. claude HIGH-2 contract: EnableDebugLogging must be
// observed immediately by the next IsDebugMode call. Argv-derived predicates
// are recomputed each call — cheap (≤8 argv scans of typical 50-flag argv).
func IsDebugMode() bool {
	if runtimeDebugEnabled.Load() {
		return true
	}
	if isEnvTruthy(os.Getenv("DEBUG")) ||
		isEnvTruthy(os.Getenv("DEBUG_SDK")) ||
		isEnvTruthy(os.Getenv("OPENDBX_DEBUG")) {
		return true
	}
	argv := processArgv()
	for _, arg := range argv {
		switch {
		case arg == "--debug",
			arg == "-d",
			arg == "--debug-to-stderr",
			arg == "-d2e":
			return true
		case strings.HasPrefix(arg, "--debug="),
			strings.HasPrefix(arg, "--debug-file="),
			arg == "--debug-file":
			return true
		}
	}
	return false
}

// getMinDebugLogLevel resolves the minimum level from ENV
// OPENDBX_DEBUG_LOG_LEVEL, falling back to LevelDebug.
//
// CC parity (debug.ts:34-40): CLAUDE_CODE_DEBUG_LOG_LEVEL → opendbx ENV
// name swapped per spec-0.4 Q4* (OPENDBX_* prefix convention).
func getMinDebugLogLevel() Level {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv("OPENDBX_DEBUG_LOG_LEVEL")))
	if raw == "" {
		return LevelDebug
	}
	if lvl, err := ParseLevel(raw); err == nil {
		return lvl
	}
	return LevelDebug
}

// isEnvTruthy mirrors CC's envUtils.isEnvTruthy: "true" / "1" / "yes" /
// "on" all truthy (case-insensitive); everything else (including "false",
// "0", "no", "off", "", whitespace) is falsy.
func isEnvTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "yes", "on":
		return true
	default:
		return false
	}
}
