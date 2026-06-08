// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package bootstrap

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/sqlrush/opendbx/internal/domain/db"
	// postgres driver is registered by the production drivers.go side-effect
	// import (spec-1.19 R-fix); tests rely on that, not a test-only import.
	"github.com/sqlrush/opendbx/internal/platform/config"
	"github.com/sqlrush/opendbx/internal/platform/logger"
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

// TestPostgresDriverRegisteredInProduction guards against the post-impl
// codex HIGH-1 regression: the postgres driver must be registered by the
// PRODUCTION side-effect import (drivers.go), NOT a test-only blank import.
// This file no longer imports the postgres package, so a passing Lookup here
// proves the production wiring registers it.
func TestPostgresDriverRegisteredInProduction(t *testing.T) {
	if _, ok := db.Lookup("postgres"); !ok {
		t.Fatal("postgres driver not registered — production bootstrap/drivers.go side-effect import is missing (codex HIGH-1)")
	}
}

// TestOpenConnection covers the end-to-end composition path including the
// single sanctioned secret.Expose() boundary (post-impl go-reviewer HIGH —
// OpenConnection was 0% covered).
func TestOpenConnection(t *testing.T) {
	ctx := context.Background()

	// 0 connections → CONN.NO_CONNECTION (ActiveConnection error path).
	if _, err := OpenConnection(ctx, &config.Config{}, ""); !errors.Is(err, ErrNoConnection) {
		t.Errorf("empty cfg → want CONN.NO_CONNECTION, got %v", err)
	}

	// fields mode on a non-DSNComposer driver → CONN.UNSUPPORTED_FIELDS
	// (ResolveDSN error path).
	cfgNC := &config.Config{Connections: []config.ConnectionConfig{
		{Alias: "nc", Driver: "fake-nocompose", Host: "h", Database: "d", User: "u"},
	}}
	if _, err := OpenConnection(ctx, cfgNC, "nc"); !errors.Is(err, ErrUnsupportedFields) {
		t.Errorf("non-composer fields → want CONN.UNSUPPORTED_FIELDS, got %v", err)
	}

	// DSN mode against the real postgres driver → reaches db.Open via the
	// single secret.Expose() boundary; a dead port yields a sanitized DB.*
	// error (the point is the boundary executes, not the connection succeeds).
	cfgDSN := &config.Config{Connections: []config.ConnectionConfig{
		{Alias: "pg", Driver: "postgres", DSN: "postgres://u:pw@127.0.0.1:1/db?sslmode=disable&connect_timeout=1"},
	}}
	_, err := OpenConnection(ctx, cfgDSN, "pg")
	if err == nil {
		t.Error("OpenConnection to a dead port should fail")
	}
	if strings.Contains(err.Error(), "pw") {
		t.Errorf("OpenConnection error leaked password: %s", err.Error())
	}
}

// --- spec-2.3a D-4: db_query registration ---

// TestDBQueryExecutors_NoConnection — zero connections → not registered.
func TestDBQueryExecutors_NoConnection(t *testing.T) {
	t.Parallel()
	if got := DBQueryExecutors(&config.Config{}); got != nil {
		t.Errorf("no connection → %v; want nil (not registered)", got)
	}
}

// TestDBQueryExecutors_Ambiguous — multiple connections, no default → not
// registered (the differentiated-reason path; codex/cr MED-2).
func TestDBQueryExecutors_Ambiguous(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{Connections: []config.ConnectionConfig{
		{Alias: "a", Driver: "postgres", Host: "h", Database: "d", User: "u"},
		{Alias: "b", Driver: "postgres", Host: "h", Database: "d", User: "u"},
	}}
	if got := DBQueryExecutors(cfg); got != nil {
		t.Errorf("ambiguous → %v; want nil (not registered)", got)
	}
}

// TestDBQueryExecutors_Registered — a selectable connection registers exactly
// one db_query executor WITHOUT opening it (startup is DB-I/O-free).
func TestDBQueryExecutors_Registered(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{Connections: []config.ConnectionConfig{
		{Alias: "only", Driver: "postgres", Host: "h", Database: "d", User: "u"},
	}}
	got := DBQueryExecutors(cfg)
	if len(got) != 1 || got[0].Name() != "db_query" {
		t.Fatalf("registered = %v; want one db_query executor", got)
	}
}

// TestConnUnavailableReason — each ActiveConnection error maps to a distinct
// actionable reason (codex/cr MED-2).
func TestConnUnavailableReason(t *testing.T) {
	t.Parallel()
	_, noneErr := ActiveConnection(&config.Config{}, "")
	_, ambErr := ActiveConnection(&config.Config{Connections: []config.ConnectionConfig{
		{Alias: "a"}, {Alias: "b"},
	}}, "")
	none := connUnavailableReason(noneErr)
	amb := connUnavailableReason(ambErr)
	if none == amb {
		t.Errorf("no-connection (%q) and ambiguous (%q) reasons must differ", none, amb)
	}
	if !strings.Contains(amb, "default_connection") {
		t.Errorf("ambiguous reason should hint default_connection: %q", amb)
	}
}

// TestWarnIfInsecureSSL_FileOnly — the warning must reach the debug file
// only, never stderr, so it cannot tear the TUI cell grid under
// --debug-to-stderr (spec-2.3a codex MED-3). NOT parallel: mutates
// os.Stderr + the logger global.
func TestWarnIfInsecureSSL_FileOnly(t *testing.T) {
	logPath := t.TempDir() + "/ssl.log"
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	defer func() { os.Stderr = oldStderr }()

	if err := logger.Init(logger.InitInput{SessionID: "ssl", LogPath: logPath, DebugToStderr: true}); err != nil {
		t.Fatalf("logger.Init: %v", err)
	}
	warnIfInsecureSSL(config.ConnectionConfig{Alias: "a", SSLMode: "disable"})

	if err := w.Close(); err != nil {
		t.Fatalf("close pipe: %v", err)
	}
	stderrRaw, _ := io.ReadAll(r)
	if strings.Contains(string(stderrRaw), "TLS") {
		t.Errorf("insecure-SSL warning tore the TUI via stderr: %q", stderrRaw)
	}
	// It must still be recorded in the debug file.
	fileRaw, _ := os.ReadFile(logPath)
	if !strings.Contains(string(fileRaw), "TLS") {
		t.Errorf("warning missing from debug file:\n%s", fileRaw)
	}
}
