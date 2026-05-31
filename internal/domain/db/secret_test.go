// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package db

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

const secretRaw = "postgres://admin:sup3rs3cr3t@db.internal:5432/prod?sslmode=require"

// secretSinks renders a SecretDSN through every common leak path. None may
// contain the raw password/host.
func secretSinks(s SecretDSN) map[string]string {
	jsonBytes, _ := json.Marshal(struct{ DSN SecretDSN }{s})
	return map[string]string{
		"String":       s.String(),
		"%v":           fmt.Sprintf("%v", s),
		"%s":           fmt.Sprintf("%s", s),
		"%q":           fmt.Sprintf("%q", s),
		"%#v":          fmt.Sprintf("%#v", s),
		"%x":           fmt.Sprintf("%x", s),
		"fmt.Errorf":   fmt.Errorf("open failed: %w or %v", fmt.Errorf("x"), s).Error(),
		"json.Marshal": string(jsonBytes),
	}
}

func TestSecretDSNNeverLeaks(t *testing.T) {
	s := NewSecretDSN(secretRaw)
	for sink, out := range secretSinks(s) {
		for _, leak := range []string{"sup3rs3cr3t", "db.internal", "admin", "prod"} {
			if strings.Contains(out, leak) {
				t.Errorf("sink %s leaked %q: %s", sink, leak, out)
			}
		}
		if !strings.Contains(out, "REDACTED") {
			t.Errorf("sink %s did not render redaction placeholder: %s", sink, out)
		}
	}
}

func TestSecretDSNExpose(t *testing.T) {
	s := NewSecretDSN(secretRaw)
	if s.Expose() != secretRaw {
		t.Errorf("Expose() = %q, want raw DSN", s.Expose())
	}
}

func TestSecretDSNGoString(t *testing.T) {
	// fmt's %#v routes through Format (which shadows GoString), so exercise
	// GoString directly to confirm it too is leak-safe.
	g := NewSecretDSN(secretRaw).GoString()
	if strings.Contains(g, "sup3rs3cr3t") || !strings.Contains(g, "REDACTED") {
		t.Errorf("GoString leaked or missing placeholder: %s", g)
	}
}

func TestSecretDSNIsZero(t *testing.T) {
	if !(SecretDSN{}).IsZero() {
		t.Error("zero SecretDSN should report IsZero")
	}
	if NewSecretDSN("x").IsZero() {
		t.Error("non-empty SecretDSN should not report IsZero")
	}
}
