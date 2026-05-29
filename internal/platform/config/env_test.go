// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package config

import (
	"strings"
	"testing"
	"time"
)

func TestEnvMap_HasExpectedEntries(t *testing.T) {
	m := EnvMap()
	want := map[string]string{
		"OPENDBX_SECURITY_DEFAULT_LEVEL":       "Security.DefaultLevel",
		"OPENDBX_LLM_REQUEST_TIMEOUT":          "LLM.RequestTimeout",
		"OPENDBX_LLM_API_KEY":                  "LLM.APIKey",
		"OPENDBX_OUTPUT_FORMAT":                "Output.Format",
		"OPENDBX_SESSION_MAX_HISTORY_MESSAGES": "Session.MaxHistoryMessages",
		"OPENDBX_TRACE_SAMPLE_RATE":            "Trace.SampleRate",
		"OPENDBX_DIAGNOSE_MAX_TURNS":           "Diagnose.MaxTurns",
		"OPENDBX_DIAGNOSE_TOOL_TIMEOUT":        "Diagnose.ToolTimeout",
		"OPENDBX_DIAGNOSE_TOTAL_TIMEOUT":       "Diagnose.TotalTimeout",
	}
	for envName, path := range want {
		if got := m[envName]; got != path {
			t.Errorf("EnvMap[%s] = %q, want %q", envName, got, path)
		}
	}
}

func TestApplyENV_String(t *testing.T) {
	cfg := Default()
	t.Setenv("OPENDBX_LLM_ACTIVE_MODEL", "claude-sonnet-4-6")
	if err := applyENV(cfg); err != nil {
		t.Fatalf("applyENV: %v", err)
	}
	if cfg.LLM.ActiveModel != "claude-sonnet-4-6" {
		t.Errorf("got %q", cfg.LLM.ActiveModel)
	}
}

// TestApplyENV_DiagnoseOverrides covers all three Diagnose fields' env
// overrides — int (MaxTurns), duration (ToolTimeout), duration
// (TotalTimeout) — proving that spec-1.21 D-6 user-config knobs reach
// the runtime through the reflective env walker.
func TestApplyENV_DiagnoseOverrides(t *testing.T) {
	cfg := Default()
	t.Setenv("OPENDBX_DIAGNOSE_MAX_TURNS", "8")
	t.Setenv("OPENDBX_DIAGNOSE_TOOL_TIMEOUT", "45s")
	t.Setenv("OPENDBX_DIAGNOSE_TOTAL_TIMEOUT", "5m")
	if err := applyENV(cfg); err != nil {
		t.Fatalf("applyENV: %v", err)
	}
	if cfg.Diagnose.MaxTurns != 8 {
		t.Errorf("MaxTurns = %d; want 8", cfg.Diagnose.MaxTurns)
	}
	if cfg.Diagnose.ToolTimeout != 45*time.Second {
		t.Errorf("ToolTimeout = %v; want 45s", cfg.Diagnose.ToolTimeout)
	}
	if cfg.Diagnose.TotalTimeout != 5*time.Minute {
		t.Errorf("TotalTimeout = %v; want 5m", cfg.Diagnose.TotalTimeout)
	}
}

func TestApplyENV_Duration(t *testing.T) {
	cfg := Default()
	t.Setenv("OPENDBX_LLM_REQUEST_TIMEOUT", "60s")
	if err := applyENV(cfg); err != nil {
		t.Fatalf("applyENV: %v", err)
	}
	if cfg.LLM.RequestTimeout != 60*time.Second {
		t.Errorf("got %v", cfg.LLM.RequestTimeout)
	}
}

func TestApplyENV_Bool(t *testing.T) {
	cfg := Default()
	t.Setenv("OPENDBX_SECURITY_CONFIRM_ON_DANGEROUS", "false")
	if err := applyENV(cfg); err != nil {
		t.Fatalf("applyENV: %v", err)
	}
	if cfg.Security.ConfirmOnDangerous != false {
		t.Errorf("got %v", cfg.Security.ConfirmOnDangerous)
	}
}

func TestApplyENV_Int(t *testing.T) {
	cfg := Default()
	t.Setenv("OPENDBX_SESSION_MAX_HISTORY_MESSAGES", "50")
	if err := applyENV(cfg); err != nil {
		t.Fatalf("applyENV: %v", err)
	}
	if cfg.Session.MaxHistoryMessages != 50 {
		t.Errorf("got %d", cfg.Session.MaxHistoryMessages)
	}
}

func TestApplyENV_Float(t *testing.T) {
	cfg := Default()
	t.Setenv("OPENDBX_TRACE_SAMPLE_RATE", "0.5")
	if err := applyENV(cfg); err != nil {
		t.Fatalf("applyENV: %v", err)
	}
	if cfg.Trace.SampleRate != 0.5 {
		t.Errorf("got %v", cfg.Trace.SampleRate)
	}
}

func TestApplyENV_Slice(t *testing.T) {
	cfg := Default()
	t.Setenv("OPENDBX_SENTINEL_NOTIFY_CHANNELS", "slack,email,webhook")
	if err := applyENV(cfg); err != nil {
		t.Fatalf("applyENV: %v", err)
	}
	if len(cfg.Sentinel.NotifyChannels) != 3 || cfg.Sentinel.NotifyChannels[1] != "email" {
		t.Errorf("got %v", cfg.Sentinel.NotifyChannels)
	}
}

func TestApplyENV_BadDurationFails(t *testing.T) {
	cfg := Default()
	t.Setenv("OPENDBX_LLM_REQUEST_TIMEOUT", "not-a-duration")
	err := applyENV(cfg)
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !strings.Contains(err.Error(), "OPENDBX_LLM_REQUEST_TIMEOUT") {
		t.Errorf("error should mention env name: %v", err)
	}
}

func TestApplyENV_NoTagFieldIgnored(t *testing.T) {
	// 'sources' field has no env tag and shouldn't blow up.
	cfg := Default()
	if err := applyENV(cfg); err != nil {
		t.Fatalf("applyENV: %v", err)
	}
}

func TestApplyENV_UnknownVarSilentlyIgnored(t *testing.T) {
	t.Setenv("OPENDBX_DOES_NOT_EXIST", "whatever")
	cfg := Default()
	if err := applyENV(cfg); err != nil {
		t.Fatalf("applyENV should not fail on unmapped ENV: %v", err)
	}
}
