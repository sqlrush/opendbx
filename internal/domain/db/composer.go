// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File composer.go — DSNComposer capability (spec-1.19 D-2). A driver MAY
// implement DSNComposer to build a DSN from structured connection fields; the
// postgres driver does (it owns the postgres DSN dialect). Drivers that don't
// implement it require the user to supply a full DSN string (the bootstrap
// resolver returns CONN.UNSUPPORTED_FIELDS for fields-mode on such a driver).
//
// This is a capability interface (composition of Driver), mirroring spec-1.18's
// decision to extend Conn via QueryConn rather than by adding methods to the
// frozen interface — adding ComposeDSN to Driver itself would break every
// non-postgres driver stub.
//
// Design: spec-1.19-connection-config.

package db

// ConnFields is the structured form of a connection (the non-DSN mode). The
// config layer fills it from ConnectionConfig + the resolved password; the
// driver composes a SecretDSN from it.
type ConnFields struct {
	Host     string
	Port     int
	Database string
	User     string
	Password string
	SSLMode  string
}

// DSNComposer is an optional driver capability: compose a SecretDSN from
// structured fields. A driver that implements it supports the fields mode of
// connection config; one that doesn't requires a full DSN string.
type DSNComposer interface {
	Driver
	// ComposeDSN builds a SecretDSN from structured fields. Any build/parse
	// error MUST be sanitized (never wrap an error whose text could embed the
	// password/DSN).
	ComposeDSN(f ConnFields) (SecretDSN, error)
}
