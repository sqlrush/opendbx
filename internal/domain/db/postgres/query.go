// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File query.go — pgConn's read-only Query capability (spec-2.3a D-2),
// making *pgConn satisfy db.QueryConn.
//
// Read-only is enforced SERVER-side: every statement runs inside a
// `BeginTx(AccessMode: ReadOnly)` transaction that is ALWAYS rolled back.
// Persistent-table writes / DDL raise SQLSTATE 25006 (→ DB.READONLY_
// VIOLATION via classify). This is NOT a zero-side-effect guarantee —
// sequence advancement, session advisory locks, temp tables, and remote
// (dblink/FDW) effects escape a read-only transaction; the real boundary
// is a least-privilege connection role (spec-2.3a § 1 / R-5).
//
// The pgx.Tx / pgx.Rows surfaces are consumed through the narrow queryTx /
// queryRows interfaces so unit fakes implement a handful of methods rather
// than pgx's full 11-method Tx / 9-method Rows (spec-2.3a Q10 / cr HIGH-3).

package postgres

import (
	"context"
	"database/sql/driver"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/sqlrush/opendbx/internal/domain/db"
)

// Compile-time: *pgConn satisfies the query capability (spec-1.18 R-8).
var _ db.QueryConn = (*pgConn)(nil)

// queryTx is the narrow tx surface pgConn.Query uses. *pgx.Tx satisfies it.
type queryTx interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Rollback(ctx context.Context) error
}

// queryRows is the narrow rows surface pgConn.Query uses. pgx.Rows satisfies it.
type queryRows interface {
	FieldDescriptions() []pgconn.FieldDescription
	Next() bool
	Values() ([]any, error)
	Err() error
	Close()
}

// Query runs ONE read-only SQL statement and returns a bounded text page.
// ctx cancel/deadline is propagated through classify, which preserves the
// ctx root under Unwrap so the tool layer's errors.Is honors the
// spec-1.21 two-track contract (spec-2.3a D-1 / codex CRIT-1).
func (c *pgConn) Query(ctx context.Context, sql string, opts db.QueryOptions) (db.QueryResult, error) {
	rawTx, err := c.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		// errcode-lint:exempt -- spec-1.18 D-4: classify returns a sanitized db.Err* (or nil); ctx root preserved under Unwrap.
		return db.QueryResult{}, classify(err)
	}
	var tx queryTx = rawTx
	// Always roll back — a read-only tx has nothing to commit, and rollback
	// is the zero-effect close for the persistent-write/DDL set (residual
	// session effects are documented, spec-2.3a R-5). Rollback after a
	// committed/closed tx is a harmless no-op in pgx.
	//
	// WithoutCancel: if the query was cancelled/timed-out, rolling back with
	// the SAME cancelled ctx makes pgx fail the ROLLBACK wire send and HARD-
	// CLOSE (die) the pooled conn, discarding it. A detached ctx lets ROLLBACK
	// complete so the conn returns cleanly to the pool (post-impl go MED-1).
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	rawRows, err := tx.Query(ctx, sql)
	if err != nil {
		// errcode-lint:exempt -- spec-1.18 D-4: classify returns a sanitized db.Err*; 25006 → DB.READONLY_VIOLATION.
		return db.QueryResult{}, classify(err)
	}
	var rows queryRows = rawRows
	defer rows.Close()

	return scanBounded(rows, opts)
}

// scanBounded reads up to MaxRows rows plus one sentinel to decide
// truncation, rendering each cell to text. A mid-iteration Values() error
// breaks immediately; rows.Err() then carries the cause (spec-2.3a cr
// MED-3 — no partial nil rows emitted).
func scanBounded(rows queryRows, opts db.QueryOptions) (db.QueryResult, error) {
	limit := opts.EffectiveMaxRows()
	maxCell := opts.EffectiveMaxCellRunes()

	fields := rows.FieldDescriptions()
	cols := make([]string, len(fields))
	for i, f := range fields {
		cols[i] = f.Name
	}

	res := db.QueryResult{Columns: cols}
	for rows.Next() {
		if len(res.Rows) == limit {
			// One more row exists beyond the cap → truncated. Stop fetching.
			res.RowsTruncated = true
			break
		}
		vals, err := rows.Values()
		if err != nil {
			break // rows.Err() carries it; do not emit a partial row
		}
		row := make([]string, len(vals))
		for i, v := range vals {
			text, truncated := formatCell(v, maxCell)
			row[i] = text
			if truncated {
				res.CellsTruncated = true
			}
		}
		res.Rows = append(res.Rows, row)
	}
	if err := rows.Err(); err != nil {
		// errcode-lint:exempt -- spec-1.18 D-4: classify returns a sanitized db.Err*.
		return db.QueryResult{}, classify(err)
	}
	return res, nil
}

// formatCell renders one pgx value to a stable text cell and reports
// whether it was rune-truncated (spec-2.3a D-2 pinned type table; cr/codex
// HIGH-2). byte-stability is load-bearing for dedup keys + goldens.
func formatCell(v any, maxRunes int) (text string, truncated bool) {
	s := renderValue(v)
	r := []rune(s)
	if len(r) > maxRunes {
		return string(r[:maxRunes]) + "…", true
	}
	return s, false
}

// renderValue is the type table (spec-2.3a D-2 pinned; cr/codex HIGH-2).
// Explicit scalar cases keep output byte-stable and avoid fmt.Sprint traps:
// floats must NOT print in scientific notation (pgx returns bare float64 for
// float8 — e.g. pg_stat checkpoint_write_time would render "3.6e+06" without
// FormatFloat 'f', post-impl go MED-2). driver.Valuer covers the pgtype
// family (pgtype.Numeric.Value() → canonical decimal string).
func renderValue(v any) string {
	switch x := v.(type) {
	case nil:
		return "NULL"
	case string:
		return x
	case []byte:
		return "\\x" + hex.EncodeToString(x) // bytea-style, not "[97 98]"
	case time.Time:
		return x.Format(time.RFC3339Nano)
	case bool:
		return strconv.FormatBool(x)
	case int:
		return strconv.FormatInt(int64(x), 10)
	case int16:
		return strconv.FormatInt(int64(x), 10)
	case int32:
		return strconv.FormatInt(int64(x), 10)
	case int64:
		return strconv.FormatInt(x, 10)
	case uint32: // pgx OID type
		return strconv.FormatUint(uint64(x), 10)
	case float32:
		return strconv.FormatFloat(float64(x), 'f', -1, 32)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64) // "3600000" not "3.6e+06"
	case driver.Valuer:
		dv, err := x.Value()
		if err != nil || dv == nil {
			return fmt.Sprintf("%v", v)
		}
		if b, ok := dv.([]byte); ok {
			return "\\x" + hex.EncodeToString(b)
		}
		if s, ok := dv.(string); ok { // pgtype.Numeric → decimal string
			return s
		}
		return fmt.Sprintf("%v", dv)
	default:
		return fmt.Sprintf("%v", v) // unknown — stable best-effort
	}
}
