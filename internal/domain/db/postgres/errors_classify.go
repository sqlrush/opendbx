// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File errors_classify.go — map pgx / network / context errors onto the
// shared db.Err* taxonomy (spec-1.18 D-4). This mapping is PG-specific and
// private to this package; each driver family owns its own classify (MySQL /
// Oracle use entirely different error-code schemes).
//
// SECURITY (spec-1.18 R-3, three-route review HIGH): pgxpool.New / pgconn
// parse errors can embed the DSN — including the password — in their Error()
// text. errcode.Wrap would render that wrapped text via Error(), leaking it
// to logs and stderr. So classify returns a *safeErr whose Error() renders
// ONLY the safe "[DB.*] message", while its Unwrap chain still preserves the
// pgx root for errors.Is / errors.As (so callers can match
// context.DeadlineExceeded, *pgconn.PgError, or a db.Err* sentinel).

package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/sqlrush/opendbx/internal/domain/db"
	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

// safeErr is a sanitized error: Error() exposes only the structured
// [DB.*] message+hint; the pgx root is preserved for matching but never
// rendered. spec-1.18 D-4.
type safeErr struct {
	structured errcode.Error // [DB.*] message — no wrapped, safe to render/log
	root       error         // pgx / net / ctx root — matched, never rendered
}

// Error renders only the safe structured text (no pgx/DSN text).
func (e *safeErr) Error() string { return e.structured.Error() }

// Unwrap returns both the structured errcode error (so errcode.As / a
// db.Err* errors.Is match) and the pgx root (so errors.As(&pgErr) /
// errors.Is(ctx.Err) match). Go 1.20+ multi-error unwrap. root is always
// non-nil here (sanitized is only called from classify on a non-nil err); a
// nil entry would simply be skipped by errors.Is/As traversal regardless.
func (e *safeErr) Unwrap() []error {
	return []error{e.structured, e.root}
}

// sanitized builds a safeErr for a registered DB.* code, preserving root for
// matching while keeping it out of the rendered text. The empty msg/hint make
// errcode.New inherit the registered db.Err* defaults (errcode.New fallback) —
// so the rendered "[DB.*] message" stays the canonical sentinel text.
func sanitized(code string, root error) error {
	return &safeErr{structured: errcode.New(code, "", ""), root: root}
}

// classify maps a pgx/network/context error onto a sanitized db.Err* error.
// Order matters: timeout/cancel is checked before SQLSTATE so a cancelled
// query is TIMEOUT, not QUERY_FAILED. nil in → nil out.
func classify(err error) error {
	if err == nil {
		return nil
	}
	// Timeout / cancellation first (covers ctx and net.Error timeouts; pgconn
	// wraps these). Before the SQLSTATE switch and the generic fallback.
	if pgconn.Timeout(err) ||
		errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, context.Canceled) {
		return sanitized(db.ErrTimeout.Code(), err)
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if len(pgErr.Code) < 2 { // malformed / empty SQLSTATE: a server-side error, generic
			return sanitized(db.ErrQueryFailed.Code(), err) // len guard: Code[:2] must not panic
		}
		// Code-specific overrides where the SQLSTATE class is misleading.
		switch pgErr.Code {
		case "42501": // insufficient_privilege — an authz failure, not a query bug
			return sanitized(db.ErrAuthFailed.Code(), err)
		case "3D000": // invalid_catalog_name — target database does not exist
			return sanitized(db.ErrConnectFailed.Code(), err)
		case "25006": // read_only_sql_transaction — a write/DDL hit the db_query
			// READ ONLY tx. This is the ONLY 25-class code mapped specially
			// (spec-2.3a D-2 / cr HIGH-1): 0A000 (feature_not_supported) is
			// NOT a read-only signal and stays QUERY_FAILED via the default.
			return sanitized(db.ErrReadOnlyViolation.Code(), err)
		}
		switch pgErr.Code[:2] { // SQLSTATE class
		case "28": // invalid_authorization_specification / invalid_password
			return sanitized(db.ErrAuthFailed.Code(), err)
		case "08": // connection_exception
			return sanitized(db.ErrConnectFailed.Code(), err)
		case "53", "57": // insufficient_resources / operator_intervention
			return sanitized(db.ErrUnavailable.Code(), err)
		default: // 42 syntax / 3D other / 23 integrity / 25 (non-25006) txn-state / ...
			// opendbx: 25-class other than 25006 (e.g. 25001 active_sql_transaction)
			// falls through to QUERY_FAILED — a simplification; 25006 is handled by
			// the override above (spec-2.3a ❌-11 / cr LOW-1).
			return sanitized(db.ErrQueryFailed.Code(), err)
		}
	}

	// Not a PgError and not a timeout: a dial / DNS / refused / parse error.
	// These are connection-phase failures (and the ones whose text may embed
	// the DSN — handled by safeErr).
	return sanitized(db.ErrConnectFailed.Code(), err)
}
