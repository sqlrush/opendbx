// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package bootstrap

import (
	"context"
	"errors"
	"testing"

	"github.com/sqlrush/opendbx/internal/domain/db"
	_ "github.com/sqlrush/opendbx/internal/domain/db/postgres" // register postgres driver
	"github.com/sqlrush/opendbx/internal/platform/config"
)

// noComposeDriver implements db.Driver but NOT db.DSNComposer.
type noComposeDriver struct{}

var errStubOpen = errors.New("noComposeDriver.Open is not exercised by tests")

func (noComposeDriver) Name() string { return "fake-nocompose" }
func (noComposeDriver) Open(context.Context, string) (db.Conn, error) {
	return nil, errStubOpen // never called; sentinel keeps the linter (nilnil) happy
}

func init() { db.Register(noComposeDriver{}) }

func TestActiveConnection(t *testing.T) {
	one := config.ConnectionConfig{Alias: "only", Driver: "postgres", DSN: "postgres://x"}
	two := config.ConnectionConfig{Alias: "second", Driver: "postgres", DSN: "postgres://y"}

	// 0 connections → NO_CONNECTION
	if _, err := ActiveConnection(&config.Config{}, ""); !errors.Is(err, ErrNoConnection) {
		t.Errorf("empty → want CONN.NO_CONNECTION, got %v", err)
	}
	// 1 connection, no selector → auto
	if c, err := ActiveConnection(&config.Config{Connections: []config.ConnectionConfig{one}}, ""); err != nil || c.Alias != "only" {
		t.Errorf("single auto = (%v, %v)", c.Alias, err)
	}
	// N connections, no selector → AMBIGUOUS
	if _, err := ActiveConnection(&config.Config{Connections: []config.ConnectionConfig{one, two}}, ""); !errors.Is(err, ErrAmbiguous) {
		t.Errorf("multi → want CONN.AMBIGUOUS, got %v", err)
	}
	// CLI alias wins
	cfg := &config.Config{Connections: []config.ConnectionConfig{one, two}}
	if c, err := ActiveConnection(cfg, "second"); err != nil || c.Alias != "second" {
		t.Errorf("cli alias = (%v, %v)", c.Alias, err)
	}
	// default_connection
	cfg.DefaultConnection = "only"
	if c, err := ActiveConnection(cfg, ""); err != nil || c.Alias != "only" {
		t.Errorf("default_connection = (%v, %v)", c.Alias, err)
	}
	// unknown alias
	if _, err := ActiveConnection(cfg, "nope"); !errors.Is(err, ErrUnknownAlias) {
		t.Errorf("unknown → want CONN.UNKNOWN_ALIAS, got %v", err)
	}
}

func TestResolveDSN(t *testing.T) {
	// DSN passthrough
	s, err := ResolveDSN(config.ConnectionConfig{Driver: "postgres", DSN: "postgres://raw"})
	if err != nil || s.Expose() != "postgres://raw" {
		t.Errorf("passthrough = (%q, %v)", s.Expose(), err)
	}
	// fields mode (postgres composes)
	s, err = ResolveDSN(config.ConnectionConfig{Driver: "postgres", Host: "h", Database: "d", User: "u", Password: "p"})
	if err != nil {
		t.Fatalf("fields compose: %v", err)
	}
	if s.IsZero() {
		t.Error("composed SecretDSN is zero")
	}
	// fields mode with a non-DSNComposer driver → UNSUPPORTED_FIELDS
	_, err = ResolveDSN(config.ConnectionConfig{Driver: "fake-nocompose", Host: "h", Database: "d", User: "u"})
	if !errors.Is(err, ErrUnsupportedFields) {
		t.Errorf("non-composer → want CONN.UNSUPPORTED_FIELDS, got %v", err)
	}
	// unknown driver → DB.DRIVER_UNKNOWN
	_, err = ResolveDSN(config.ConnectionConfig{Driver: "no-such", Host: "h", Database: "d", User: "u"})
	if !errors.Is(err, db.ErrDriverUnknown) {
		t.Errorf("unknown driver → want DB.DRIVER_UNKNOWN, got %v", err)
	}
}

func TestWarnIfInsecureSSLNoPanic(t *testing.T) {
	// logger is a no-op when uninitialised; just verify no panic across modes.
	for _, m := range []string{"", "disable", "allow", "prefer", "require", "verify-full"} {
		warnIfInsecureSSL(config.ConnectionConfig{Alias: "a", SSLMode: m})
	}
	warnIfInsecureSSL(config.ConnectionConfig{Alias: "a", DSN: "postgres://x"}) // DSN mode skipped
}
