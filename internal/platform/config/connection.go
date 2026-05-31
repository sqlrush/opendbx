// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File connection.go — connection lookup + credential resolution (spec-1.19
// D-4). The password is resolved env-over-config: OPENDBX_DB_PASSWORD_<ALIAS>
// wins over the config `password` field. The env var is dynamic (alias-keyed)
// so it cannot use a static `env:` tag — PasswordSource exposes its provenance
// for `admin config sources` without ever revealing the value.
//
// Design: spec-1.19-connection-config.

package config

import (
	"os"
	"strings"
)

// passwordEnvPrefix is the fixed prefix of the per-alias password env VAR
// NAME (not a credential value).
//
//nolint:gosec // spec-1.19 D-4: G101 false positive — env-var name prefix, not a secret value.
const passwordEnvPrefix = "OPENDBX_DB_PASSWORD_"

// envKey normalises an alias into the env-var suffix: uppercase, with every
// character outside [A-Z0-9] replaced by '_'. Two aliases that normalise to
// the same key would collide; validateConnections rejects such configs so this
// transform is injective over a validated config.
func envKey(alias string) string {
	var b strings.Builder
	b.Grow(len(alias))
	for _, r := range strings.ToUpper(alias) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

// passwordEnvVar returns the full env-var name for an alias's password.
func passwordEnvVar(alias string) string { return passwordEnvPrefix + envKey(alias) }

// ResolvePassword returns the effective password for a connection:
// OPENDBX_DB_PASSWORD_<ALIAS> env var if set, else the config field. Never
// logs the value.
func (c ConnectionConfig) ResolvePassword() string {
	if v, ok := os.LookupEnv(passwordEnvVar(c.Alias)); ok {
		return v
	}
	return c.Password
}

// ConnectionByAlias returns a COPY of the connection with the given alias and
// true, or the zero value and false. A copy (not a pointer into the slice) so
// callers cannot mutate the live config (规则 12 immutability).
func (c *Config) ConnectionByAlias(alias string) (ConnectionConfig, bool) {
	for i := range c.Connections {
		if c.Connections[i].Alias == alias {
			return c.Connections[i], true // value copy
		}
	}
	return ConnectionConfig{}, false
}

// PasswordSource reports where a connection's password comes from — SourceENV
// (the per-alias env var is set), the connections-section source (a config
// file), or SourceDefault (no password). It never reveals the value; it is for
// `admin config sources` provenance.
func (c *Config) PasswordSource(alias string) SettingSource {
	conn, ok := c.ConnectionByAlias(alias)
	if !ok {
		return SourceDefault
	}
	if _, env := os.LookupEnv(passwordEnvVar(conn.Alias)); env {
		return SourceENV
	}
	if conn.Password != "" {
		return c.Source("Connections")
	}
	return SourceDefault
}
