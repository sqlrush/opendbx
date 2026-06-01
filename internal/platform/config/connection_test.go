// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package config

import "testing"

func TestEnvKey(t *testing.T) {
	cases := map[string]string{
		"prod":    "PROD",
		"prod-db": "PROD_DB",
		"prod_db": "PROD_DB", // collides with prod-db (validateConnections rejects)
		"my.db 1": "MY_DB_1",
		"Mixed99": "MIXED99",
	}
	for in, want := range cases {
		if got := envKey(in); got != want {
			t.Errorf("envKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolvePassword(t *testing.T) {
	conn := ConnectionConfig{Alias: "prod", Password: "from-config"}

	if got := conn.ResolvePassword(); got != "from-config" {
		t.Errorf("config password = %q, want from-config", got)
	}

	t.Setenv("OPENDBX_DB_PASSWORD_PROD", "from-env")
	if got := conn.ResolvePassword(); got != "from-env" {
		t.Errorf("env override = %q, want from-env", got)
	}
}

func TestConnectionByAliasReturnsCopy(t *testing.T) {
	cfg := &Config{Connections: []ConnectionConfig{{Alias: "a", Host: "orig"}}}
	conn, ok := cfg.ConnectionByAlias("a")
	if !ok {
		t.Fatal("alias a not found")
	}
	conn.Host = "mutated" // mutate the copy
	if cfg.Connections[0].Host != "orig" {
		t.Error("mutating the returned ConnectionConfig changed the live config (规则 12)")
	}

	if _, ok := cfg.ConnectionByAlias("missing"); ok {
		t.Error("missing alias reported found")
	}
}

func TestPasswordSource(t *testing.T) {
	cfg := &Config{Connections: []ConnectionConfig{
		{Alias: "withpw", Password: "x"},
		{Alias: "nopw"},
	}}
	cfg.SetSource("Connections", SourceUserSettings)

	if got := cfg.PasswordSource("nopw"); got != SourceDefault {
		t.Errorf("no-password source = %v, want SourceDefault", got)
	}
	if got := cfg.PasswordSource("withpw"); got != SourceUserSettings {
		t.Errorf("config-password source = %v, want SourceUserSettings", got)
	}
	t.Setenv("OPENDBX_DB_PASSWORD_NOPW", "y")
	if got := cfg.PasswordSource("nopw"); got != SourceENV {
		t.Errorf("env-password source = %v, want SourceENV", got)
	}
	if got := cfg.PasswordSource("missing"); got != SourceDefault {
		t.Errorf("missing alias source = %v, want SourceDefault", got)
	}
}
