// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Cross-platform path resolution for opendbx config sources (spec-0.4 D-5).
//
// Conventions:
//   - Linux: $XDG_CONFIG_HOME/opendbx/config.yaml, fallback ~/.config/opendbx/config.yaml
//   - macOS: ~/Library/Application Support/opendbx/config.yaml,
//     fallback ~/.opendbx/config.yaml (matches opendb 老版)
//   - Windows: %APPDATA%/opendbx/config.yaml
//
// `--settings <path>` flag overrides the user-tier path entirely (becomes
// SourceFlagSettings, NOT SourceUserSettings).

package config

import (
	"os"
	"path/filepath"
	"runtime"
)

// SourcePaths bundles the on-disk paths that Load() reads in priority order
// (low → high; later overrides earlier).
type SourcePaths struct {
	PolicyPath  string // managed-settings.yaml — admin scope
	UserPath    string // ~/.opendbx/config.yaml — user scope
	ProjectPath string // ./.opendbx/config.yaml — project shared
	LocalPath   string // ./.opendbx/local.yaml — project gitignored
	FlagPath    string // --settings <path> override (set by Load if non-empty)
}

// DefaultSourcePaths returns the file paths resolved per platform conventions.
// `cwd` is the project working directory (typically os.Getwd()); when empty,
// project/local paths are skipped.
//
// Returns an error if HOME directory is unresolvable (no fallback — per
// spec § 3.2 Load() fails-fast on environment issues).
func DefaultSourcePaths(cwd string) (SourcePaths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return SourcePaths{}, err
	}

	var policy, user string
	switch runtime.GOOS {
	case "darwin":
		// macOS: prefer Library/Application Support; fallback to ~/.opendbx for
		// 1:1 parity with opendb 老版.
		userLib := filepath.Join(home, "Library", "Application Support", "opendbx", "config.yaml")
		userDot := filepath.Join(home, ".opendbx", "config.yaml")
		if fileExists(userLib) {
			user = userLib
		} else {
			user = userDot
		}
		policy = "/etc/opendbx/managed.yaml"
	case "windows":
		appdata := os.Getenv("APPDATA")
		if appdata == "" {
			appdata = filepath.Join(home, "AppData", "Roaming")
		}
		user = filepath.Join(appdata, "opendbx", "config.yaml")
		programdata := os.Getenv("PROGRAMDATA")
		if programdata == "" {
			programdata = "C:\\ProgramData"
		}
		policy = filepath.Join(programdata, "opendbx", "managed.yaml")
	default: // linux + other unix
		xdg := os.Getenv("XDG_CONFIG_HOME")
		if xdg == "" {
			xdg = filepath.Join(home, ".config")
		}
		user = filepath.Join(xdg, "opendbx", "config.yaml")
		policy = "/etc/opendbx/managed.yaml"
	}

	var project, local string
	if cwd != "" {
		project = filepath.Join(cwd, ".opendbx", "config.yaml")
		local = filepath.Join(cwd, ".opendbx", "local.yaml")
	}

	return SourcePaths{
		PolicyPath:  policy,
		UserPath:    user,
		ProjectPath: project,
		LocalPath:   local,
	}, nil
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
