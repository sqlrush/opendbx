// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File query.go — the QueryConn query CAPABILITY over Conn (spec-2.3a
// D-1). This realizes the spec-1.18 R-8 forward design: query arrives as
// a COMPOSED interface, never by adding a method to the frozen db.Conn
// (which would break every fake and future driver).
//
// § 3.7 multi-DB: the minimal common result shape is a text page. Each
// driver renders its own native types to strings driver-side (a single
// any-typed row set would leak every driver's type system upward). MySQL/
// Oracle/openGauss implement QueryConn later against the SAME contract.

package db

import "context"

// Default result caps (spec-2.3a Q6, R2 user decision). Package-level
// constants — config knobs are deferred (spec-2.3a ❌-3).
const (
	// DefaultMaxRows bounds rows kept before truncation. The driver fetches
	// MaxRows+1 to distinguish "exactly MaxRows" from "more existed".
	DefaultMaxRows = 100
	// DefaultMaxCellRunes bounds per-cell rune length before truncation.
	DefaultMaxCellRunes = 200
)

// QueryConn is the OPTIONAL query capability over Conn. Acquire via a type
// assertion: qc, ok := conn.(db.QueryConn). The caller MUST Close() the
// underlying Conn when the assertion FAILS — a real driver may return a
// live pool from Open before it implements QueryConn (spec-2.3a D-3 /
// codex HIGH-2: not closing leaks the pool on every retry).
type QueryConn interface {
	Conn
	// Query executes ONE read-only SQL statement inside a server-side READ
	// ONLY transaction and returns the bounded, text-rendered page.
	// Enforcement is SERVER-side (persistent-table writes / DDL → SQLSTATE
	// 25006); callers must NOT rely on client-side SQL inspection. The ctx
	// deadline is the only v1 cancellation mechanism. Implementations MUST
	// let ctx cancel/deadline remain matchable via errors.Is on the
	// returned error (so the tool layer can honor the spec-1.21 two-track
	// timeout/cancel contract rather than treating it as a recoverable DB
	// error) — the postgres driver satisfies this because its sanitized
	// classify preserves the ctx root under Unwrap.
	Query(ctx context.Context, sql string, opts QueryOptions) (QueryResult, error)
}

// QueryOptions bounds a single Query. A zero field falls back to the
// Default* constant, so QueryOptions{} is the canonical "use defaults".
type QueryOptions struct {
	MaxRows      int // rows kept before truncation; 0 → DefaultMaxRows
	MaxCellRunes int // per-cell rune cap; 0 → DefaultMaxCellRunes
}

// QueryResult is a fully text-rendered result page.
//
// Ownership (spec-2.3a D-1, arch HIGH-2): the driver does NOT retain or
// mutate the result after Query returns — the caller owns it. This is NOT
// a deep-copy guarantee: Rows is [][]string whose backing arrays are not
// cloned, so callers must not assume mutating one returned result is
// isolated from another. (No consumer needs that; documenting it prevents
// a false copy-isolation assumption.)
type QueryResult struct {
	// Columns are the result column names in query order.
	Columns []string
	// Rows are text cells (driver rendered each native value to a string;
	// a NULL DB value renders as the literal "NULL").
	Rows [][]string
	// RowsTruncated is true iff a MaxRows+1 sentinel row existed — i.e. the
	// query produced strictly more than MaxRows rows. Exactly MaxRows rows
	// leaves this false.
	RowsTruncated bool
	// CellsTruncated is true iff at least one cell hit MaxCellRunes.
	CellsTruncated bool
}

// EffectiveMaxRows resolves the row cap (0 → default). Pure helper so the
// driver and tests agree on the fallback.
func (o QueryOptions) EffectiveMaxRows() int {
	if o.MaxRows <= 0 {
		return DefaultMaxRows
	}
	return o.MaxRows
}

// EffectiveMaxCellRunes resolves the per-cell cap (0 → default).
func (o QueryOptions) EffectiveMaxCellRunes() int {
	if o.MaxCellRunes <= 0 {
		return DefaultMaxCellRunes
	}
	return o.MaxCellRunes
}
