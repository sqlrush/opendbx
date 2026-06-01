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

// newPool is the pool constructor seam. Production = pgxpool.New (returning a
// *pgxpool.Pool that satisfies pgxPool); tests override it to inject a fake so
// Open's success path is unit-testable without a real PostgreSQL (spec-1.18
// R-fix; post-impl go-reviewer MED-1).
//
//nolint:gochecknoglobals // spec-1.18 R-fix: test seam, mirrors the database/sql driver-constructor pattern.
var newPool = func(ctx context.Context, dsn string) (pgxPool, error) {
	return pgxpool.New(ctx, dsn)
}

// Name returns the registry key.
func (Driver) Name() string { return "postgres" }

// Open builds a pgxpool from the DSN and verifies reachability (eager-ping)
// before returning. Any failure is classified to a sanitized DB.* error that
// never renders the DSN.
//
// The caller MUST supply a context with a deadline (e.g. context.WithTimeout)
// — the eager Ping has no internal timeout floor, so a black-holed host would
// otherwise block until the OS TCP timeout (post-impl security-reviewer LOW-1).
func (Driver) Open(ctx context.Context, dsn string) (db.Conn, error) {
	pool, err := newPool(ctx, dsn)
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
