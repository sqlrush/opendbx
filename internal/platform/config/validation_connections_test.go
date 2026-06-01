// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package config

import (
	"errors"
	"strings"
	"testing"
)

// validateConns runs the full validator and returns the connection-related
// ValidationErrors (by rule prefix) for assertions.
func validateConns(t *testing.T, conns []ConnectionConfig) ValidationErrors {
	t.Helper()
	cfg := &Config{Connections: conns}
	err := Validate(cfg)
	if err == nil {
		return nil
	}
	var ve ValidationErrors
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationErrors, got %T: %v", err, err)
	}
	return ve
}

func hasRule(es ValidationErrors, rule string) bool {
	for _, e := range es {
		if e.Rule == rule {
			return true
		}
	}
	return false
}

func TestValidateConnectionsXOR(t *testing.T) {
	// both DSN and fields → conflict
	es := validateConns(t, []ConnectionConfig{
		{Alias: "a", Driver: "postgres", DSN: "postgres://x", Host: "h", Database: "d", User: "u"},
	})
	if !hasRule(es, "dsn-xor-fields") {
		t.Errorf("both-set should fail dsn-xor-fields; got %v", es)
	}
	// neither → conflict
	es = validateConns(t, []ConnectionConfig{{Alias: "a", Driver: "postgres"}})
	if !hasRule(es, "dsn-xor-fields") {
		t.Errorf("neither-set should fail dsn-xor-fields; got %v", es)
	}
	// DSN-only → ok
	if es := validateConns(t, []ConnectionConfig{{Alias: "a", Driver: "postgres", DSN: "postgres://x"}}); hasRule(es, "dsn-xor-fields") {
		t.Errorf("DSN-only should pass XOR; got %v", es)
	}
	// full fields → ok
	if es := validateConns(t, []ConnectionConfig{{Alias: "a", Driver: "postgres", Host: "h", Database: "d", User: "u"}}); hasRule(es, "dsn-xor-fields") {
		t.Errorf("full fields should pass XOR; got %v", es)
	}
}

func TestValidateConnectionsRequiredTrio(t *testing.T) {
	es := validateConns(t, []ConnectionConfig{{Alias: "a", Driver: "postgres", Host: "h"}}) // missing db+user
	if !hasRule(es, "required-in-fields-mode") {
		t.Errorf("fields mode missing database/user should fail; got %v", es)
	}
}

func TestValidateConnectionsPortRange(t *testing.T) {
	es := validateConns(t, []ConnectionConfig{{Alias: "a", Driver: "postgres", Host: "h", Database: "d", User: "u", Port: 70000}})
	if !hasRule(es, "range") {
		t.Errorf("port 70000 should fail range; got %v", es)
	}
	// port 0 (unset) is allowed
	if es := validateConns(t, []ConnectionConfig{{Alias: "a", Driver: "postgres", Host: "h", Database: "d", User: "u"}}); hasRule(es, "range") {
		t.Errorf("port 0 should be allowed; got %v", es)
	}
}

func TestValidateConnectionsAliasUnique(t *testing.T) {
	es := validateConns(t, []ConnectionConfig{
		{Alias: "dup", Driver: "postgres", DSN: "postgres://1"},
		{Alias: "dup", Driver: "postgres", DSN: "postgres://2"},
	})
	if !hasRule(es, "unique") {
		t.Errorf("duplicate alias should fail unique; got %v", es)
	}
}

func TestValidateConnectionsEnvKeyCollision(t *testing.T) {
	es := validateConns(t, []ConnectionConfig{
		{Alias: "prod-db", Driver: "postgres", DSN: "postgres://1"},
		{Alias: "prod_db", Driver: "postgres", DSN: "postgres://2"},
	})
	if !hasRule(es, "envkey-unique") {
		t.Errorf("prod-db vs prod_db should fail envkey-unique; got %v", es)
	}
}

func TestValidateConnectionsSSLModeOneof(t *testing.T) {
	es := validateConns(t, []ConnectionConfig{
		{Alias: "a", Driver: "postgres", Host: "h", Database: "d", User: "u", SSLMode: "bogus"},
	})
	if !hasRule(es, "oneof") {
		t.Errorf("bogus sslmode should fail oneof; got %v", es)
	}
}

// TestValidateConnectionsNoPasswordLeak: a failing connection that also carries
// a password must not echo the password value in any ValidationError.
func TestValidateConnectionsNoPasswordLeak(t *testing.T) {
	es := validateConns(t, []ConnectionConfig{
		{Alias: "a", Driver: "postgres", DSN: "postgres://x", Host: "h", Password: "leakpw123"},
	})
	for _, e := range es {
		if strings.Contains(e.Error(), "leakpw123") {
			t.Errorf("ValidationError leaked password: %s", e.Error())
		}
	}
}
