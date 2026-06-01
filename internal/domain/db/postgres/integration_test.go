// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

//go:build pg_integration

// File integration_test.go — real-PostgreSQL smoke (spec-1.18 D-8). PARKED
// behind the pg_integration build tag AND the OPENDBX_TEST_PG_DSN env var so
// it never runs in the default CI gate (Docker / a real server is not
// available there). It DOES compile in the default gate via
// `go test -tags pg_integration ./...` (build-only) to catch pgx API drift.
//
// Run locally against a real PG:
//
//	OPENDBX_TEST_PG_DSN='postgres://user:pass@localhost:5432/postgres' \
//	  go test -tags pg_integration ./internal/domain/db/postgres/...

package postgres

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sqlrush/opendbx/internal/domain/db"
)

func dsnOrSkip(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("OPENDBX_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("OPENDBX_TEST_PG_DSN not set; skipping real-PG integration")
	}
	return dsn
}

func TestIntegrationOpenPingHealthClose(t *testing.T) {
	dsn := dsnOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := db.Open(ctx, "postgres", dsn)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if err := conn.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	h, err := conn.HealthCheck(ctx)
	if err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
	if !h.Reachable || !strings.Contains(strings.ToLower(h.Version), "postgresql") {
		t.Errorf("Health = %+v", h)
	}
	if h.Latency <= 0 {
		t.Errorf("latency = %v, want > 0", h.Latency)
	}
}

// distinctiveSecret is a memorable password used in the parked integration
// DSNs so a no-leak assertion is independently meaningful (post-impl
// security-reviewer MED-1 — the prior 1-char "p" was too weak a signal).
const distinctiveSecret = "S3cr3tPass_DoNotLeak"

func TestIntegrationBadHostNoLeak(t *testing.T) {
	dsnOrSkip(t) // ensure the suite is enabled
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	dsn := "postgres://u:" + distinctiveSecret + "@127.0.0.1:1/db?sslmode=disable&connect_timeout=2"
	_, err := db.Open(ctx, "postgres", dsn)
	if err == nil {
		t.Fatal("Open bad host returned nil error")
	}
	// connect failure or timeout — both acceptable; must not leak the password.
	if strings.Contains(err.Error(), distinctiveSecret) || strings.Contains(err.Error(), "password") {
		t.Errorf("error leaked credentials: %s", err.Error())
	}
}

// TestIntegrationConnectRefusedNoLeak exercises a connect-refused failure (the
// 127.0.0.1:1 dead port never reaches PostgreSQL, so this is a connection-phase
// failure, NOT a SQLSTATE 3D000 path — the 3D000 mapping is covered by the unit
// classify table). The point here is the real-pgxpool error path stays
// password-free.
func TestIntegrationConnectRefusedNoLeak(t *testing.T) {
	dsnOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	dsn := "postgres://u:" + distinctiveSecret + "@127.0.0.1:1/anydb?sslmode=disable&connect_timeout=2"
	_, err := db.Open(ctx, "postgres", dsn)
	if err == nil {
		t.Fatal("Open connect-refused returned nil error")
	}
	if strings.Contains(err.Error(), distinctiveSecret) || strings.Contains(err.Error(), "password") {
		t.Errorf("error leaked credentials: %s", err.Error())
	}
}
