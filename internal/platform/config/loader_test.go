// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

// helpers used across loader/paths/env tests
func mkdir(p string) error { return os.MkdirAll(p, 0o750) }
func writeFile(p, body string) error {
	return os.WriteFile(p, []byte(body), 0o600)
}

func TestLoad_NoSources_UsesDefaults(t *testing.T) {
	tmp := t.TempDir()
	cfg, err := Load(LoadOptions{CWD: tmp})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Output.Format != "text" {
		t.Errorf("expected default format text, got %q", cfg.Output.Format)
	}
}

func TestLoad_ProjectOverridesUser(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // isolate from real ~/.opendbx
	cwd := t.TempDir()
	mustMkdir(t, filepath.Join(cwd, ".opendbx"))
	mustWrite(t, filepath.Join(cwd, ".opendbx", "config.yaml"), `
output:
  format: json
`)
	cfg, err := Load(LoadOptions{CWD: cwd})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Output.Format != "json" {
		t.Errorf("project override failed; got %q", cfg.Output.Format)
	}
	if cfg.Source("Output") != SourceProjectSettings {
		t.Errorf("Source = %v, want SourceProjectSettings", cfg.Source("Output"))
	}
}

func TestLoad_LocalOverridesProject(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cwd := t.TempDir()
	mustMkdir(t, filepath.Join(cwd, ".opendbx"))
	mustWrite(t, filepath.Join(cwd, ".opendbx", "config.yaml"), `
output:
  format: json
`)
	mustWrite(t, filepath.Join(cwd, ".opendbx", "local.yaml"), `
output:
  format: stream-json
`)
	cfg, err := Load(LoadOptions{CWD: cwd})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Output.Format != "stream-json" {
		t.Errorf("local override failed; got %q", cfg.Output.Format)
	}
	if cfg.Source("Output") != SourceLocalSettings {
		t.Errorf("Source = %v, want SourceLocalSettings", cfg.Source("Output"))
	}
}

func TestLoad_ENVOverridesProject(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cwd := t.TempDir()
	mustMkdir(t, filepath.Join(cwd, ".opendbx"))
	mustWrite(t, filepath.Join(cwd, ".opendbx", "config.yaml"), `
output:
  format: json
`)
	t.Setenv("OPENDBX_OUTPUT_FORMAT", "stream-json")
	cfg, err := Load(LoadOptions{CWD: cwd})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Output.Format != "stream-json" {
		t.Errorf("ENV override failed; got %q", cfg.Output.Format)
	}
	if cfg.Source("Output") != SourceENV {
		t.Errorf("Source = %v, want SourceENV", cfg.Source("Output"))
	}
}

func TestLoad_FlagSettingsOverridesAll(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cwd := t.TempDir()
	flagFile := filepath.Join(cwd, "custom.yaml")
	mustWrite(t, flagFile, `
output:
  format: json
`)
	cfg, err := Load(LoadOptions{CWD: cwd, FlagSettingsPath: flagFile})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Output.Format != "json" {
		t.Errorf("flag-settings load failed; got %q", cfg.Output.Format)
	}
}

func TestLoad_InlineJSONSettings(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := Load(LoadOptions{
		CWD:              t.TempDir(),
		FlagSettingsPath: `{"output":{"format":"json"}}`,
	})
	if err != nil {
		t.Fatalf("Load inline settings: %v", err)
	}
	if cfg.Output.Format != "json" {
		t.Errorf("inline --settings JSON did not apply; got %q", cfg.Output.Format)
	}
	if cfg.Source("Output.Format") != SourceFlagSettings {
		t.Errorf("Source(Output.Format) = %v, want SourceFlagSettings", cfg.Source("Output.Format"))
	}
}

func TestLoad_FlagSettingsMissingFails(t *testing.T) {
	cfg, err := Load(LoadOptions{FlagSettingsPath: "/nonexistent/foo.yaml"})
	if err == nil {
		t.Fatal("Load should fail when --settings file missing")
	}
	if !errors.Is(err, ErrLoadFailed) {
		t.Fatalf("Load missing --settings err = %v, want CONFIG.LOAD_FAILED", err)
	}
	if cfg != nil {
		t.Error("Load returned non-nil cfg on error")
	}
}

func TestLoad_BadYAMLFailsImmediately(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cwd := t.TempDir()
	mustMkdir(t, filepath.Join(cwd, ".opendbx"))
	mustWrite(t, filepath.Join(cwd, ".opendbx", "config.yaml"),
		"output:\n  format: : invalid yaml :\n")
	_, err := Load(LoadOptions{CWD: cwd})
	if err == nil {
		t.Fatal("Load should fail on bad YAML")
	}
	if !errors.Is(err, ErrLoadFailed) {
		t.Fatalf("bad YAML err = %v, want CONFIG.LOAD_FAILED", err)
	}
}

func TestLoad_UnknownFieldFails(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cwd := t.TempDir()
	mustMkdir(t, filepath.Join(cwd, ".opendbx"))
	mustWrite(t, filepath.Join(cwd, ".opendbx", "config.yaml"),
		"unknown_section:\n  enabled: true\n")
	_, err := Load(LoadOptions{CWD: cwd})
	if err == nil {
		t.Fatal("Load should fail with KnownFields(true) on unknown section")
	}
	if !strings.Contains(err.Error(), "unknown_section") {
		t.Errorf("error should mention unknown_section: %v", err)
	}
	if !errors.Is(err, ErrLoadFailed) {
		t.Fatalf("unknown field err = %v, want CONFIG.LOAD_FAILED", err)
	}
}

func TestLoad_FailsValidation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cwd := t.TempDir()
	mustMkdir(t, filepath.Join(cwd, ".opendbx"))
	mustWrite(t, filepath.Join(cwd, ".opendbx", "config.yaml"),
		"output:\n  format: yaml\n") // not in oneof
	_, err := Load(LoadOptions{CWD: cwd})
	if err == nil {
		t.Fatal("Load should fail validation on bad format")
	}
	if !errors.Is(err, ErrValidationFailed) {
		t.Fatalf("validation err = %v, want CONFIG.VALIDATION_FAILED", err)
	}
}

func TestLoad_FlagOverridesENV(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("OPENDBX_OUTPUT_FORMAT", "json")
	cfg, err := Load(LoadOptions{
		CWD: t.TempDir(),
		FlagOverrides: []FieldOverride{
			{Path: "Output.Format", Value: "stream-json"},
		},
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Output.Format != "stream-json" {
		t.Errorf("CLI flag should override ENV; got %q", cfg.Output.Format)
	}
	if cfg.Source("Output") != SourceCLIFlag {
		t.Errorf("Source = %v, want SourceCLIFlag", cfg.Source("Output"))
	}
}

func TestLoad_LargeFileRejected(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cwd := t.TempDir()
	mustMkdir(t, filepath.Join(cwd, ".opendbx"))
	big := strings.Repeat("a", 1<<20+1) // 1MB+1
	mustWrite(t, filepath.Join(cwd, ".opendbx", "config.yaml"), big)
	_, err := Load(LoadOptions{CWD: cwd})
	if err == nil {
		t.Fatal("Load should reject >1MB yaml")
	}
	if !errors.Is(err, ErrLoadFailed) {
		t.Fatalf("large file err = %v, want CONFIG.LOAD_FAILED", err)
	}
}

func TestLoad_FieldLevelSourcesDoNotBleedAcrossSiblings(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cwd := t.TempDir()
	mustMkdir(t, filepath.Join(cwd, ".opendbx"))
	mustWrite(t, filepath.Join(cwd, ".opendbx", "config.yaml"), `
output:
  format: json
`)
	t.Setenv("OPENDBX_OUTPUT_COLOR", "never")
	cfg, err := Load(LoadOptions{CWD: cwd})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Source("Output.Format") != SourceProjectSettings {
		t.Errorf("Source(Output.Format) = %v, want SourceProjectSettings", cfg.Source("Output.Format"))
	}
	if cfg.Source("Output.Color") != SourceENV {
		t.Errorf("Source(Output.Color) = %v, want SourceENV", cfg.Source("Output.Color"))
	}
}

func TestLoad_SettingSourcesRestrictsUserProjectLocal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cwd := t.TempDir()
	mustMkdir(t, filepath.Join(cwd, ".opendbx"))
	mustWrite(t, filepath.Join(cwd, ".opendbx", "config.yaml"), `
output:
  format: json
`)
	cfg, err := Load(LoadOptions{CWD: cwd, SettingSources: "user"})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Output.Format != "text" {
		t.Errorf("project settings should be skipped; got output.format=%q", cfg.Output.Format)
	}
}

func TestLoad_SettingSourcesInvalidFails(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, err := Load(LoadOptions{CWD: t.TempDir(), SettingSources: "user,banana"})
	if err == nil {
		t.Fatal("expected invalid --setting-sources to fail")
	}
	if !strings.Contains(err.Error(), "banana") {
		t.Errorf("error should mention invalid token: %v", err)
	}
	var ec errcode.Error
	if !errors.As(err, &ec) || ec.Code() != ErrLoadFailed.Code() {
		t.Fatalf("invalid setting-sources errcode = %v / %q, want %s", err, ec.Code(), ErrLoadFailed.Code())
	}
}

// helpers
func mustMkdir(t *testing.T, p string) {
	t.Helper()
	if err := mkdir(p); err != nil {
		t.Fatal(err)
	}
}
func mustWrite(t *testing.T, p, body string) {
	t.Helper()
	if err := writeFile(p, body); err != nil {
		t.Fatal(err)
	}
}
