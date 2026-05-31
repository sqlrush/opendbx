// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File postgres.go — the PostgreSQL driver (spec-1.18 D-2). pgx/v5 via
// pgxpool: concurrency-safe with automatic reconnect, hidden behind the
// db.Conn interface. Open is eager (builds the pool then Pings) so an
// unreachable target fails at Open with a clear DB.* code rather than on
// first use — important for spec-1.19 instance switching (catalog S-5).
//
// Design: spec-1.18-pg-driver.

package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sqlrush/opendbx/internal/domain/db"
)

// Driver is the postgres implementation of db.Driver.
type Driver struct{}

// Compile-time interface conformance.
var _ db.Driver = Driver{}

// Name returns the registry key.
func (Driver) Name() string { return "postgres" }

// Open builds a pgxpool from the DSN and verifies reachability (eager-ping)
// before returning. Any failure is classified to a sanitized DB.* error that
// never renders the DSN.
func (Driver) Open(ctx context.Context, dsn string) (db.Conn, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		// errcode-lint:exempt -- spec-1.18 D-4: classify always returns a sanitized errcode.Error (db.Err*); this is the error origin, not a pass-through.
		return nil, classify(err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		// errcode-lint:exempt -- spec-1.18 D-4: classify always returns a sanitized errcode.Error (db.Err*).
		return nil, classify(err)
	}
	return &pgConn{pool: pool}, nil
}

// init registers the driver. init does register only (规则 12).
//
//nolint:gochecknoinits // spec-1.18 D-1: driver self-registration, mirrors database/sql.
func init() { db.Register(Driver{}) }
