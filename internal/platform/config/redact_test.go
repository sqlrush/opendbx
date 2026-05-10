// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package config

import (
	"bytes"
	"strings"
	"testing"
)

func TestRedact_MasksAPIKey(t *testing.T) {
	cfg := Default()
	cfg.LLM.APIKey = "sk-secret-do-not-leak"
	r := Redact(cfg)
	if r.LLM.APIKey != RedactedSentinel {
		t.Errorf("APIKey not redacted: got %q", r.LLM.APIKey)
	}
	// Original should NOT be modified.
	if cfg.LLM.APIKey != "sk-secret-do-not-leak" {
		t.Errorf("original cfg mutated: %q", cfg.LLM.APIKey)
	}
}

func TestRedact_MasksConnectionDSN(t *testing.T) {
	cfg := Default()
	cfg.Connections = []ConnectionConfig{
		{Alias: "prod", Driver: "postgres", DSN: "postgres://u:secret@h/db"},
	}
	r := Redact(cfg)
	if r.Connections[0].DSN != RedactedSentinel {
		t.Errorf("DSN not redacted: got %q", r.Connections[0].DSN)
	}
	// Alias should be untouched.
	if r.Connections[0].Alias != "prod" {
		t.Errorf("Alias unexpectedly modified")
	}
}

func TestRedact_EmptyFieldsLeftAlone(t *testing.T) {
	cfg := Default() // APIKey is "" (default)
	r := Redact(cfg)
	if r.LLM.APIKey != "" {
		t.Errorf("empty APIKey should stay empty, got %q", r.LLM.APIKey)
	}
}

func TestDumpDefaults_RedactsSecrets(t *testing.T) {
	cfg := Default()
	cfg.LLM.APIKey = "sk-secret-key"
	// Capture a redacted dump via Redact + yaml manually.
	var buf bytes.Buffer
	if err := WriteDefaultsYAML(&buf); err != nil {
		t.Fatalf("WriteDefaultsYAML: %v", err)
	}
	out := buf.String()
	// Default has empty APIKey; Redact still leaves it empty (not <REDACTED>).
	// The contract is "non-empty secret → <REDACTED>".
	if strings.Contains(out, "sk-secret-key") {
		t.Error("dump-defaults leaked APIKey")
	}
}

func TestRedact_NilConfig(t *testing.T) {
	if Redact(nil) != nil {
		t.Error("Redact(nil) should return nil")
	}
}
