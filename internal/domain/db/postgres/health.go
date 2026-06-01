// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File health.go — pgConn lifecycle + health (spec-1.18 D-3). pgConn wraps a
// connection pool and satisfies db.Conn.
//
// The pool is held behind the pgxPool interface (not *pgxpool.Pool directly)
// so the lifecycle/health paths are unit-testable with a fake, without a real
// PostgreSQL server (spec-1.18 R-5). *pgxpool.Pool satisfies pgxPool.
//
// Design: spec-1.18-pg-driver.

package postgres

import (
	"context"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/sqlrush/opendbx/internal/domain/db"
)

// pgxPool is the subset of *pgxpool.Pool that pgConn uses. Kept minimal so a
// fake can stand in for unit tests.
type pgxPool interface {
	Ping(ctx context.Context) error
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Close()
}

// pgConn is a live postgres connection (pool) behind db.Conn.
type pgConn struct {
	pool      pgxPool
	closeOnce sync.Once // makes Close idempotent (pgxpool.Close is not re-entrant)
}

// Compile-time interface conformance.
var _ db.Conn = (*pgConn)(nil)

// Ping verifies the connection is alive, honouring ctx.
func (c *pgConn) Ping(ctx context.Context) error {
	// errcode-lint:exempt -- spec-1.18 D-4: classify always returns a sanitized errcode.Error (db.Err*) or nil.
	return classify(c.pool.Ping(ctx))
}

// HealthCheck probes the server with SELECT version(), returning the version
// string and round-trip latency. On failure it returns the zero Health and a
// sanitized DB.* error (the error is authoritative; Health is not consulted).
func (c *pgConn) HealthCheck(ctx context.Context) (db.Health, error) {
	start := time.Now()
	var version string
	if err := c.pool.QueryRow(ctx, "SELECT version()").Scan(&version); err != nil {
		// errcode-lint:exempt -- spec-1.18 D-4: classify always returns a sanitized errcode.Error (db.Err*).
		return db.Health{}, classify(err)
	}
	return db.Health{Reachable: true, Version: version, Latency: time.Since(start)}, nil
}

// Close releases the pool. It is idempotent and takes no context (pgxpool.Close
// blocks until connections are returned and cannot be cancelled).
func (c *pgConn) Close() error {
	c.closeOnce.Do(func() { c.pool.Close() })
	return nil
}
