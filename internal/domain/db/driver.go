// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File driver.go — provider-agnostic database driver interface (spec-1.18
// D-1). The minimal common set across PG / MySQL / Oracle / openGauss
// (§ 3.7 multi-DB matrix): connection lifecycle + liveness + a version
// health probe. NOTHING query-shaped — query execution arrives in spec-2.1
// as a CAPABILITY interface (QueryConn = Conn + Query...), never by adding
// methods to Conn (that would be a Go interface break for every fake and
// future driver).
//
// Design: spec-1.18-pg-driver.

package db

import (
	"context"
	"time"
)

// Driver constructs connections for one database family. Implementations
// register themselves via Register in their package init.
type Driver interface {
	// Name is the registry key, e.g. "postgres".
	Name() string
	// Open establishes a usable connection from an opaque DSN. The DSN is
	// driver-specific and parsed by the driver (spec-1.19 owns DSN
	// composition / credential handling). Open is eager: it verifies
	// reachability before returning, so an unreachable target fails here
	// rather than on first use.
	Open(ctx context.Context, dsn string) (Conn, error)
}

// Conn is a live connection (or pool) to one database instance. FROZEN to
// lifecycle + health for spec-1.18; query capability is added separately in
// spec-2.1 via a composed QueryConn interface.
type Conn interface {
	// Ping verifies the connection is alive, honouring ctx.
	Ping(ctx context.Context) error
	// HealthCheck runs a lightweight server probe (server version + round
	// trip latency). On failure it returns the zero Health and a non-nil
	// error — the error is authoritative, the Health value is not consulted.
	HealthCheck(ctx context.Context) (Health, error)
	// Close releases all underlying connections. It takes no context:
	// pgxpool.Close (and most pool Close implementations) block until
	// connections are returned and cannot be cancelled. Close is idempotent.
	Close() error
}

// Health is the result of a HealthCheck probe.
type Health struct {
	// Reachable is true only when the probe query succeeded.
	Reachable bool
	// Version is the server version string (e.g. from SELECT version()).
	Version string
	// Latency is the round-trip duration of the probe query.
	Latency time.Duration
}
