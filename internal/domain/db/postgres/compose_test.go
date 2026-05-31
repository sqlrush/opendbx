// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package postgres

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/sqlrush/opendbx/internal/domain/db"
)

func TestComposeDSN(t *testing.T) {
	tests := []struct {
		name   string
		fields db.ConnFields
		// substrings that must parse-survive in the EXPOSED dsn
		wantHost string
		wantUser string
		wantSSL  string
	}{
		{
			name:     "full",
			fields:   db.ConnFields{Host: "h", Port: 5433, Database: "d", User: "u", Password: "p", SSLMode: "require"},
			wantHost: "h:5433", wantUser: "u", wantSSL: "require",
		},
		{
			name:     "default port + sslmode",
			fields:   db.ConnFields{Host: "h", Database: "d", User: "u", Password: "p"},
			wantHost: "h:5432", wantUser: "u", wantSSL: "prefer",
		},
		{
			name:     "no password",
			fields:   db.ConnFields{Host: "h", Database: "d", User: "u"},
			wantHost: "h:5432", wantUser: "u", wantSSL: "prefer",
		},
		{
			name:     "ipv6 host",
			fields:   db.ConnFields{Host: "::1", Database: "d", User: "u", Password: "p"},
			wantHost: "[::1]:5432", wantUser: "u", wantSSL: "prefer",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, err := Driver{}.ComposeDSN(tc.fields)
			if err != nil {
				t.Fatalf("ComposeDSN: %v", err)
			}
			cfg, err := pgconn.ParseConfig(s.Expose()) // pgx must accept it
			if err != nil {
				t.Fatalf("pgconn.ParseConfig rejected composed DSN: %v", err)
			}
			if cfg.User != tc.wantUser {
				t.Errorf("user = %q, want %q", cfg.User, tc.wantUser)
			}
			// SecretDSN must not leak the password even here.
			if !strings.Contains(s.String(), "REDACTED") {
				t.Errorf("SecretDSN.String leaked: %s", s.String())
			}
		})
	}
}

// TestComposeDSNSpecialCharsPassword verifies percent-encoding survives a
// password containing DSN-significant characters.
func TestComposeDSNSpecialCharsPassword(t *testing.T) {
	pw := "p@ss:w/o?rd#%x"
	s, err := Driver{}.ComposeDSN(db.ConnFields{Host: "h", Database: "d", User: "u", Password: pw})
	if err != nil {
		t.Fatalf("ComposeDSN: %v", err)
	}
	cfg, err := pgconn.ParseConfig(s.Expose())
	if err != nil {
		t.Fatalf("ParseConfig rejected special-char password DSN: %v", err)
	}
	if cfg.Password != pw {
		t.Errorf("round-trip password = %q, want %q", cfg.Password, pw)
	}
}

func TestComposeDSNImplementsCapability(t *testing.T) {
	var d db.Driver = Driver{}
	if _, ok := d.(db.DSNComposer); !ok {
		t.Fatal("postgres Driver does not implement db.DSNComposer")
	}
}
