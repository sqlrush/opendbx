// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/sqlrush/opendbx/internal/domain/db"
)

func TestDriverName(t *testing.T) {
	if got := (Driver{}).Name(); got != "postgres" {
		t.Errorf("Name() = %q, want postgres", got)
	}
}

func TestDriverRegistered(t *testing.T) {
	// init() registered the driver; db.Open should find it. Use a DSN that
	// fails to parse so we exercise the registry → driver → classify path
	// without touching the network.
	_, err := db.Open(context.Background(), "postgres", "::: not a valid dsn :::")
	if err == nil {
		t.Fatal("Open with invalid DSN returned nil error")
	}
	if !errors.Is(err, db.ErrConnectFailed) {
		t.Errorf("invalid DSN: want DB.CONNECT_FAILED, got %v", err)
	}
}

func TestOpenInvalidDSNNoLeak(t *testing.T) {
	// A DSN carrying a password that fails to parse must not leak the password.
	_, err := Driver{}.Open(context.Background(), "host=x password=leakme123 invalid=")
	if err == nil {
		t.Fatal("want parse error")
	}
	if strings.Contains(err.Error(), "leakme123") {
		t.Errorf("Open error leaked password: %s", err.Error())
	}
}

// --- fake pool to unit-test pgConn lifecycle without a real PG ---

type fakeRow struct {
	scanErr error
	version string
}

func (r fakeRow) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	if len(dest) > 0 {
		if p, ok := dest[0].(*string); ok {
			*p = r.version
		}
	}
	return nil
}

type fakePool struct {
	pingErr error
	row     pgx.Row
	closes  int
	// spec-2.3a D-2: read-only query path.
	tx         pgx.Tx
	beginErr   error
	lastTxOpts pgx.TxOptions
}

func (p *fakePool) Ping(context.Context) error                       { return p.pingErr }
func (p *fakePool) QueryRow(context.Context, string, ...any) pgx.Row { return p.row }
func (p *fakePool) Close()                                           { p.closes++ }
func (p *fakePool) BeginTx(_ context.Context, o pgx.TxOptions) (pgx.Tx, error) {
	p.lastTxOpts = o
	return p.tx, p.beginErr
}

func TestPgConnPing(t *testing.T) {
	ok := &pgConn{pool: &fakePool{}}
	if err := ok.Ping(context.Background()); err != nil {
		t.Errorf("Ping ok: %v", err)
	}

	bad := &pgConn{pool: &fakePool{pingErr: &pgconn.PgError{Code: "08006"}}}
	if err := bad.Ping(context.Background()); !errors.Is(err, db.ErrConnectFailed) {
		t.Errorf("Ping err: want DB.CONNECT_FAILED, got %v", err)
	}
}

func TestPgConnHealthCheck(t *testing.T) {
	c := &pgConn{pool: &fakePool{row: fakeRow{version: "PostgreSQL 16.2"}}}
	h, err := c.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
	if !h.Reachable || h.Version != "PostgreSQL 16.2" {
		t.Errorf("Health = %+v", h)
	}
	if h.Latency < 0 {
		t.Errorf("negative latency: %v", h.Latency)
	}
}

func TestPgConnHealthCheckError(t *testing.T) {
	c := &pgConn{pool: &fakePool{row: fakeRow{scanErr: &pgconn.PgError{Code: "57P03"}}}}
	h, err := c.HealthCheck(context.Background())
	if !errors.Is(err, db.ErrUnavailable) {
		t.Errorf("want DB.UNAVAILABLE, got %v", err)
	}
	if h.Reachable {
		t.Error("Health.Reachable should be false on error (zero value)")
	}
}

// TestOpenSuccess covers Driver.Open's happy path (newPool succeeds + Ping
// succeeds → wrapped pgConn) without a real PG, via the newPool seam
// (post-impl go-reviewer MED-1).
func TestOpenSuccess(t *testing.T) {
	orig := newPool
	t.Cleanup(func() { newPool = orig })
	newPool = func(context.Context, string) (pgxPool, error) {
		return &fakePool{row: fakeRow{version: "PostgreSQL 16.2"}}, nil
	}
	conn, err := Driver{}.Open(context.Background(), "any-dsn")
	if err != nil {
		t.Fatalf("Open success: %v", err)
	}
	if conn == nil {
		t.Fatal("Open returned nil conn")
	}
	if err := conn.Ping(context.Background()); err != nil {
		t.Errorf("Ping on opened conn: %v", err)
	}
	_ = conn.Close()
}

func TestOpenPingFailsClosesPool(t *testing.T) {
	orig := newPool
	t.Cleanup(func() { newPool = orig })
	fp := &fakePool{pingErr: &pgconn.PgError{Code: "08006"}}
	newPool = func(context.Context, string) (pgxPool, error) { return fp, nil }
	_, err := Driver{}.Open(context.Background(), "dsn")
	if !errors.Is(err, db.ErrConnectFailed) {
		t.Errorf("eager-ping failure: want DB.CONNECT_FAILED, got %v", err)
	}
	if fp.closes != 1 {
		t.Errorf("pool not closed on eager-ping failure: closes=%d", fp.closes)
	}
}

func TestOpenNewPoolError(t *testing.T) {
	orig := newPool
	t.Cleanup(func() { newPool = orig })
	newPool = func(context.Context, string) (pgxPool, error) { return nil, errors.New("dial fail") }
	if _, err := (Driver{}).Open(context.Background(), "dsn"); !errors.Is(err, db.ErrConnectFailed) {
		t.Errorf("newPool error: want DB.CONNECT_FAILED, got %v", err)
	}
}

func TestPgConnCloseIdempotent(t *testing.T) {
	fp := &fakePool{}
	c := &pgConn{pool: fp}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := c.Close(); err != nil { // second Close must not panic or error
		t.Fatalf("second Close: %v", err)
	}
	if fp.closes != 1 {
		t.Errorf("pool.Close called %d times, want 1 (idempotent)", fp.closes)
	}
}
