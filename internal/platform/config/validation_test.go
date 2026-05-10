// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package config

import (
	"errors"
	"strings"
	"testing"
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
