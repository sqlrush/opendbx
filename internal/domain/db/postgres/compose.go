// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File compose.go — postgres DSNComposer capability (spec-1.19 D-3). Builds a
// postgres URL DSN from structured fields. The password is percent-encoded via
// url.UserPassword so special characters (@ : / ? # %) are safe. The result is
// a SecretDSN so it cannot be accidentally logged.
//
// SECURITY (spec-1.19 R-1): a self-check parse failure must NOT surface the raw
// DSN — composeErr returns a sanitized error with no DSN text and no wrapped
// root (the validated-field invariant makes this path unreachable in practice).
//
// Design: spec-1.19-connection-config.

package postgres

import (
	"net"
	"net/url"
	"strconv"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/sqlrush/opendbx/internal/domain/db"
	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

const (
	defaultPort    = "5432"
	defaultSSLMode = "prefer"
)

// Compile-time assertion: postgres supports the fields mode.
var _ db.DSNComposer = Driver{}

// ComposeDSN builds a postgres URL DSN from structured fields. Fields are
// assumed already validated by the config layer (host/database/user present,
// port in range, sslmode in the allowed set).
func (Driver) ComposeDSN(f db.ConnFields) (db.SecretDSN, error) {
	u := url.URL{
		Scheme: "postgres",
		Host:   net.JoinHostPort(f.Host, portOrDefault(f.Port)),
		Path:   "/" + f.Database,
	}
	if f.User != "" {
		if f.Password != "" {
			u.User = url.UserPassword(f.User, f.Password) // percent-encodes both
		} else {
			u.User = url.User(f.User)
		}
	}
	u.RawQuery = url.Values{"sslmode": {sslModeOrDefault(f.SSLMode)}}.Encode()

	raw := u.String()
	// Defensive self-check: confirm pgx (the eventual consumer) can parse it.
	// On failure, return a sanitized error WITHOUT the raw DSN — the parse
	// error text may embed the password.
	if _, err := pgconn.ParseConfig(raw); err != nil {
		// errcode-lint:exempt -- spec-1.19 D-3: sanitized; must NOT wrap the pgconn error (it embeds the DSN/password).
		return db.SecretDSN{}, errcode.New(db.ErrConnectFailed.Code(),
			"composed postgres DSN failed self-validation", "")
	}
	return db.NewSecretDSN(raw), nil
}

func portOrDefault(port int) string {
	if port <= 0 {
		return defaultPort
	}
	return strconv.Itoa(port)
}

func sslModeOrDefault(mode string) string {
	if mode == "" {
		return defaultSSLMode
	}
	return mode
}
