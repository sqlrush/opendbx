// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package config

import (
	"reflect"
	"testing"
	"time"
)

func TestDefault_NoExternalDeps(t *testing.T) {
	// Defaults must not depend on filesystem/ENV/network. Calling twice
	// must produce DeepEqual configs.
	a := Default()
	b := Default()
	if !reflect.DeepEqual(a, b) {
		t.Error("Default() called twice produced different configs")
	}
}

func TestDefault_PassesValidation(t *testing.T) {
	if err := Validate(Default()); err != nil {
		t.Errorf("Default() failed Validate: %v", err)
	}
}

func TestDefault_SecretsAreEmpty(t *testing.T) {
	d := Default()
	if d.LLM.APIKey != "" {
		t.Errorf("LLM.APIKey should be empty in Default, got %q", d.LLM.APIKey)
	}
}

func TestDefault_ReasonableValues(t *testing.T) {
	d := Default()
	if d.Session.MaxHistoryMessages < 1 {
		t.Error("MaxHistoryMessages too low")
	}
	if d.LLM.RequestTimeout != 30*time.Second {
		t.Errorf("LLM.RequestTimeout = %v, want 30s", d.LLM.RequestTimeout)
	}
	if d.Output.Format != "text" {
		t.Errorf("Output.Format = %q, want text", d.Output.Format)
	}
	if d.Scheduler.WorkerPoolSize < 1 {
		t.Error("WorkerPoolSize must be positive")
	}
	if d.Sentinel.HardCeilingFactor < 1 {
		t.Error("HardCeilingFactor must be ≥ 1")
	}
}

func TestDefault_ConnectionsAndModels_AreNil(t *testing.T) {
	d := Default()
	if d.Connections != nil {
		t.Error("Default Connections should be nil (user must add)")
	}
	if d.Models != nil {
		t.Error("Default Models should be nil (user must add)")
	}
}
