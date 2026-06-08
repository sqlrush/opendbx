// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File connection.go — connection resolution orchestration (spec-1.19 D-6/D-7).
// bootstrap is the layer that may import both platform/config and domain/db,
// so the resolution that bridges them lives here (config is the lowest layer
// and cannot import db; the postgres DSN dialect lives in the driver).
//
// Flow: ActiveConnection (pick by alias) → ResolveDSN (DSN passthrough or
// driver.ComposeDSN) → SecretDSN → db.Open(secret.Expose()). The Expose() call
// in OpenConnection is the ONLY sanctioned raw-DSN access point (spec-1.19 D-8
// grep lint).
//
// Design: spec-1.19-connection-config.

package bootstrap

import (
	"context"
	"errors"

	"github.com/sqlrush/opendbx/internal/app/diagnose"
	"github.com/sqlrush/opendbx/internal/app/tools/dbquery"
	"github.com/sqlrush/opendbx/internal/domain/db"
	"github.com/sqlrush/opendbx/internal/platform/config"
	"github.com/sqlrush/opendbx/internal/platform/errcode"
	"github.com/sqlrush/opendbx/internal/platform/logger"
)

// ActiveConnection selects the connection to use. Precedence: the CLI alias
// (--connection-alias) > config default_connection > (the sole connection if
// there is exactly one). It returns a COPY (规则 12 immutability), never a
// pointer into the live config slice.
func ActiveConnection(cfg *config.Config, cliAlias string) (config.ConnectionConfig, error) {
	alias := cliAlias
	if alias == "" {
		alias = cfg.DefaultConnection
	}
	if alias == "" {
		switch len(cfg.Connections) {
		case 0:
			return config.ConnectionConfig{}, errcode.New(ErrNoConnection.Code(), "", "")
		case 1:
			return cfg.Connections[0], nil // value copy
		default:
			return config.ConnectionConfig{}, errcode.New(ErrAmbiguous.Code(), "", "")
		}
	}
	conn, ok := cfg.ConnectionByAlias(alias)
	if !ok {
		return config.ConnectionConfig{}, errcode.Newf(ErrUnknownAlias.Code(), "未知的连接 alias: %s", alias)
	}
	return conn, nil
}

// ResolveDSN turns a connection into a SecretDSN. DSN mode is an opaque,
// self-contained passthrough (no env injection — spec-1.19 Fork B). Fields
// mode dispatches to the driver's DSNComposer capability.
func ResolveDSN(conn config.ConnectionConfig) (db.SecretDSN, error) {
	if conn.DSN != "" {
		return db.NewSecretDSN(conn.DSN), nil
	}
	drv, ok := db.Lookup(conn.Driver)
	if !ok {
		return db.SecretDSN{}, errcode.Newf(db.ErrDriverUnknown.Code(), "未注册的 driver: %s", conn.Driver)
	}
	composer, ok := drv.(db.DSNComposer)
	if !ok {
		return db.SecretDSN{}, errcode.New(ErrUnsupportedFields.Code(), "", "")
	}
	// errcode-lint:exempt -- spec-1.19 D-3: ComposeDSN returns a sanitized db.* errcode (or nil); bootstrap is a pass-through here, not the error origin.
	return composer.ComposeDSN(db.ConnFields{
		Host:     conn.Host,
		Port:     conn.Port,
		Database: conn.Database,
		User:     conn.User,
		Password: conn.ResolvePassword(),
		SSLMode:  conn.SSLMode,
	})
}

// warnIfInsecureSSL emits a visible warning when a fields-mode connection's
// effective sslmode allows an unencrypted fallback (spec-1.19 Fork C). DSN
// mode is opaque (sslmode is inside the DSN) so it is not inspected here.
func warnIfInsecureSSL(conn config.ConnectionConfig) {
	if conn.DSN != "" {
		return
	}
	mode := conn.SSLMode
	if mode == "" {
		mode = "prefer" // ComposeDSN default
	}
	switch mode {
	case "disable", "allow", "prefer":
		// file-only: spec-2.3a moved OpenConnection to a lazy tool-execution
		// path; a normal logger.Warn would tear the TUI cell grid under
		// --debug-to-stderr (codex MED-3). WarnForceFile is file-only and
		// never touches stderr (mirrors the spec-2.3 InfoForceFile precedent).
		logger.WarnForceFile("数据库连接未强制 TLS，凭据与查询可能明文传输",
			logger.Attr{Key: "alias", Value: conn.Alias},
			logger.Attr{Key: "sslmode", Value: mode},
			logger.Attr{Key: "hint", Value: "生产环境请设 sslmode=require 或更高"},
		)
	}
}

// OpenConnection is the end-to-end entry: select → resolve → warn → open. The
// single db.Open boundary is the ONLY place secret.Expose() is called.
func OpenConnection(ctx context.Context, cfg *config.Config, cliAlias string) (db.Conn, error) {
	conn, err := ActiveConnection(cfg, cliAlias)
	if err != nil {
		// errcode-lint:exempt -- spec-1.19 D-6: ActiveConnection returns a CONN.* errcode.Error; pass-through.
		return nil, err
	}
	secret, err := ResolveDSN(conn)
	if err != nil {
		// errcode-lint:exempt -- spec-1.19 D-6: ResolveDSN returns a CONN.*/DB.* errcode.Error; pass-through.
		return nil, err
	}
	warnIfInsecureSSL(conn)
	// errcode-lint:exempt -- spec-1.18 D-4: db.Open returns a sanitized db.* errcode (or nil); this is the single sanctioned secret.Expose() call site (spec-1.19 D-8).
	return db.Open(ctx, conn.Driver, secret.Expose())
}

// DBQueryExecutors returns the db_query tool executor when a connection can
// be selected from config (spec-2.3a D-4). It does NOT open the connection —
// selection only — so startup stays DB-I/O-free (the tool opens lazily on
// first Execute, spec-2.3a Q4). When no connection is usable it registers
// nothing and logs a differentiated reason (distinguishing "none configured"
// from "ambiguous — set default_connection"; codex/cr MED).
func DBQueryExecutors(cfg *config.Config) []diagnose.ToolExecutor {
	if _, err := ActiveConnection(cfg, ""); err != nil {
		logger.InfoForceFile("db_query tool not registered",
			"spec", "2.3a", "reason", connUnavailableReason(err))
		return nil
	}
	openFn := func(ctx context.Context) (db.Conn, error) {
		return OpenConnection(ctx, cfg, "")
	}
	return []diagnose.ToolExecutor{dbquery.New(openFn)}
}

// connUnavailableReason maps an ActiveConnection error to an actionable
// debug-log reason (spec-2.3a D-4 / cr MED-2: ambiguous must not look like
// "no connection").
func connUnavailableReason(err error) string {
	var ec errcode.Error
	if errors.As(err, &ec) {
		switch ec.Code() {
		case ErrNoConnection.Code():
			return "no database connection configured"
		case ErrAmbiguous.Code():
			return "multiple connections but no default_connection set; set default_connection or pass --connection-alias"
		case ErrUnknownAlias.Code():
			return "configured connection alias not found"
		}
	}
	return "connection selection failed"
}
