// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package config

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestValidate_DefaultsPass(t *testing.T) {
	if err := Validate(Default()); err != nil {
		t.Errorf("Default failed validation: %v", err)
	}
}

func TestValidate_OneofFails(t *testing.T) {
	cfg := Default()
	cfg.Output.Format = "yaml" // not in oneof
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "Output.Format") {
		t.Errorf("error should mention Output.Format: %v", err)
	}
}

func TestValidate_MinFails(t *testing.T) {
	cfg := Default()
	cfg.Session.MaxHistoryMessages = 0 // min=1
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "MaxHistoryMessages") {
		t.Errorf("error should mention MaxHistoryMessages: %v", err)
	}
}

func TestValidate_MaxFails(t *testing.T) {
	cfg := Default()
	cfg.Security.DefaultLevel = 99 // max=10
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestValidate_DedupWindowMinFails — spec-1.22: DedupWindow has validate
// "min=1" (启停 is the DedupEnabled bool, so 0 is never a valid window —
// codex HIGH-2). DedupEnabled=false does NOT exempt the field from validation.
func TestValidate_DedupWindowMinFails(t *testing.T) {
	cfg := Default()
	cfg.Diagnose.DedupWindow = 0 // min=1
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for DedupWindow=0")
	}
	if !strings.Contains(err.Error(), "DedupWindow") {
		t.Errorf("error should mention DedupWindow: %v", err)
	}
}

func TestValidate_DedupWindowMaxFails(t *testing.T) {
	cfg := Default()
	cfg.Diagnose.DedupWindow = 101 // max=100
	if err := Validate(cfg); err == nil {
		t.Fatal("expected error for DedupWindow=101")
	}
}

func TestValidate_NestedSliceErrorPath(t *testing.T) {
	cfg := Default()
	cfg.Connections = []ConnectionConfig{
		{Alias: "", Driver: "postgres", DSN: "x"}, // Alias required
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "Connections[0]") {
		t.Errorf("error should mention slice index: %v", err)
	}
}

func TestValidationErrors_RedactsSecrets(t *testing.T) {
	cfg := Default()
	cfg.LLM.APIKey = "sk-secret-key-do-not-leak"
	// Make APIKey fail something — apply an artificial rule by tampering Tier.
	cfg.LLM.Tier = "tier-99" // not in oneof
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "sk-secret-key") {
		t.Error("Validate error leaked APIKey value!")
	}
}

func TestValidate_AllErrorsAggregated(t *testing.T) {
	cfg := Default()
	cfg.Output.Format = "bogus"
	cfg.Security.DefaultLevel = 99
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error")
	}
	var verrs ValidationErrors
	if !errors.As(err, &verrs) {
		t.Fatalf("expected ValidationErrors, got %T", err)
	}
	if len(verrs) < 2 {
		t.Errorf("expected ≥ 2 aggregated errors, got %d:\n%v", len(verrs), verrs)
	}
}

func TestValidate_TraceRange(t *testing.T) {
	cfg := Default()
	cfg.Trace.SampleRate = 1.5 // max=1
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidate_RequiredOnConnectionAlias(t *testing.T) {
	cfg := Default()
	cfg.Connections = []ConnectionConfig{{Alias: "", Driver: "postgres"}}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error on missing Alias")
	}
}

// TestValidate_Diagnose_MaxTurnsBounds — the per-field MaxTurns rule
// (min=1, max=100). Spec-1.21 D-6 caps the practical upper bound to
// keep runaway loops costing the user explicit configuration intent.
func TestValidate_Diagnose_MaxTurnsBounds(t *testing.T) {
	for _, tc := range []struct {
		name    string
		val     int
		wantErr bool
	}{
		{"zero rejected", 0, true},
		{"one accepted", 1, false},
		{"hundred accepted", 100, false},
		{"hundredOne rejected", 101, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Default()
			cfg.Diagnose.MaxTurns = tc.val
			err := Validate(cfg)
			if (err == nil) == tc.wantErr {
				t.Errorf("Validate(MaxTurns=%d) err=%v; wantErr=%v", tc.val, err, tc.wantErr)
			}
		})
	}
}

// TestValidate_Diagnose_CrossFieldTimeouts — spec-1.21 D-6 invariant:
// ToolTimeout MUST be ≤ TotalTimeout (a per-tool deadline exceeding the
// loop budget is non-sensical because the loop would terminate before
// the tool could complete).
func TestValidate_Diagnose_CrossFieldTimeouts(t *testing.T) {
	cfg := Default()
	cfg.Diagnose.ToolTimeout = 11 * time.Minute // > TotalTimeout=10min
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected cross-field error when ToolTimeout > TotalTimeout")
	}
	// And a config with ToolTimeout = TotalTimeout exactly is accepted
	// (≤ is the spec-1.21 D-6 contract, not strict <).
	cfg2 := Default()
	cfg2.Diagnose.ToolTimeout = cfg2.Diagnose.TotalTimeout
	if err := Validate(cfg2); err != nil {
		t.Errorf("ToolTimeout == TotalTimeout should be accepted; got %v", err)
	}
}
