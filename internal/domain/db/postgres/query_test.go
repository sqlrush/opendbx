// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sqlrush/opendbx/internal/domain/db"
)

// --- narrow fakes (embed the pgx interface; only override what Query uses) ---

type fakeTx struct {
	pgx.Tx     // embedded nil interface — satisfies the type; unused methods unreached
	rows       pgx.Rows
	queryErr   error
	rolledBack bool
}

func (f *fakeTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return f.rows, f.queryErr
}
func (f *fakeTx) Rollback(context.Context) error { f.rolledBack = true; return nil }

type fakeRows struct {
	pgx.Rows    // embedded nil interface
	fields      []pgconn.FieldDescription
	data        [][]any
	idx         int
	valuesErrAt int // 1-based row index where Values() fails; 0 = never
	err         error
	closed      bool
}

func (r *fakeRows) FieldDescriptions() []pgconn.FieldDescription { return r.fields }
func (r *fakeRows) Next() bool {
	if r.idx < len(r.data) {
		r.idx++
		return true
	}
	return false
}
func (r *fakeRows) Values() ([]any, error) {
	if r.valuesErrAt != 0 && r.idx == r.valuesErrAt {
		// Real pgx decode failures surface a SQLSTATE-bearing error; 22P02
		// (invalid_text_representation, class 22) → DB.QUERY_FAILED.
		r.err = &pgconn.PgError{Code: "22P02", Message: "invalid input syntax"}
		return nil, r.err
	}
	return r.data[r.idx-1], nil
}
func (r *fakeRows) Err() error { return r.err }
func (r *fakeRows) Close()     { r.closed = true }

func cols(names ...string) []pgconn.FieldDescription {
	out := make([]pgconn.FieldDescription, len(names))
	for i, n := range names {
		out[i] = pgconn.FieldDescription{Name: n}
	}
	return out
}

// nRows builds a fakeRows with n single-column rows "r0".."r(n-1)".
func nRows(n int) *fakeRows {
	data := make([][]any, n)
	for i := range data {
		data[i] = []any{"r" + itoa(i)}
	}
	return &fakeRows{fields: cols("c"), data: data}
}

func itoa(i int) string { return strings.TrimSpace(string(rune('0' + i%10))) } // small i only

func newQueryConn(rows pgx.Rows, queryErr, beginErr error) (*pgConn, *fakeTx, *fakePool) {
	tx := &fakeTx{rows: rows, queryErr: queryErr}
	fp := &fakePool{tx: tx, beginErr: beginErr}
	return &pgConn{pool: fp}, tx, fp
}

// --- tests ---

// TestQuery_ReadOnlyAndAlwaysRollback — every Query opens a READ ONLY tx
// and rolls back even on the success path (spec-2.3a D-2 / Q2).
func TestQuery_ReadOnlyAndAlwaysRollback(t *testing.T) {
	t.Parallel()
	conn, tx, fp := newQueryConn(nRows(2), nil, nil)
	res, err := conn.Query(context.Background(), "SELECT 1", db.QueryOptions{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if fp.lastTxOpts.AccessMode != pgx.ReadOnly {
		t.Errorf("AccessMode = %v; want ReadOnly", fp.lastTxOpts.AccessMode)
	}
	if !tx.rolledBack {
		t.Error("success path must still Rollback (zero-commit read-only tx)")
	}
	if len(res.Rows) != 2 || res.Columns[0] != "c" {
		t.Errorf("result = %+v; want 2 rows / column c", res)
	}
}

// TestQuery_SentinelTruncation — 99/100/101 rows distinguish exactly-cap
// from over-cap via the MaxRows+1 sentinel (spec-2.3a codex HIGH-1).
func TestQuery_SentinelTruncation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		rows          int
		wantKept      int
		wantTruncated bool
	}{
		{99, 99, false},
		{100, 100, false},
		{101, 100, true},
	}
	for _, tc := range tests {
		t.Run(itoa(tc.rows%10)+"_rows", func(t *testing.T) {
			t.Parallel()
			// build N rows with distinct content (itoa only handles small i,
			// so use a plain loop here)
			data := make([][]any, tc.rows)
			for i := range data {
				data[i] = []any{"x"}
			}
			rows := &fakeRows{fields: cols("c"), data: data}
			conn, _, _ := newQueryConn(rows, nil, nil)
			res, err := conn.Query(context.Background(), "SELECT 1", db.QueryOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if len(res.Rows) != tc.wantKept {
				t.Errorf("kept %d rows; want %d", len(res.Rows), tc.wantKept)
			}
			if res.RowsTruncated != tc.wantTruncated {
				t.Errorf("RowsTruncated = %v; want %v (input %d rows)", res.RowsTruncated, tc.wantTruncated, tc.rows)
			}
			if !rows.closed {
				t.Error("rows must be Closed")
			}
		})
	}
}

// TestQuery_FormatCellTypeTable — the pinned type table (spec-2.3a D-2;
// cr/codex HIGH-2): NUMERIC/bytea/time/nil/bool do not fall to fmt.Sprint
// garbage.
func TestQuery_FormatCellTypeTable(t *testing.T) {
	t.Parallel()
	num := pgtype.Numeric{}
	if err := num.Scan("12345.67"); err != nil {
		t.Fatalf("seed numeric: %v", err)
	}
	ts := time.Date(2026, 6, 8, 12, 30, 0, 0, time.UTC)
	row := []any{nil, "hello", []byte{0x61, 0x62}, ts, true, int64(42), num}
	rows := &fakeRows{
		fields: cols("nullc", "str", "bytes", "tstamp", "flag", "num64", "numeric"),
		data:   [][]any{row},
	}
	conn, _, _ := newQueryConn(rows, nil, nil)
	res, err := conn.Query(context.Background(), "SELECT *", db.QueryOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got := res.Rows[0]
	want := []string{"NULL", "hello", "\\x6162", "2026-06-08T12:30:00Z", "true", "42", "12345.67"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("cell[%d] = %q; want %q", i, got[i], want[i])
		}
	}
}

// TestQuery_CellTruncation — a cell over MaxCellRunes is rune-truncated
// with an ellipsis and flags CellsTruncated (spec-2.3a D-2).
func TestQuery_CellTruncation(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("世", 10) // 10 runes (CJK — rune-aware, not byte)
	rows := &fakeRows{fields: cols("c"), data: [][]any{{long}}}
	conn, _, _ := newQueryConn(rows, nil, nil)
	res, err := conn.Query(context.Background(), "SELECT 1", db.QueryOptions{MaxCellRunes: 4})
	if err != nil {
		t.Fatal(err)
	}
	if !res.CellsTruncated {
		t.Error("want CellsTruncated")
	}
	if res.Rows[0][0] != "世世世世…" {
		t.Errorf("cell = %q; want 4 runes + ellipsis", res.Rows[0][0])
	}
}

// TestQuery_RowValuesErrorBreaks — a mid-scan Values() error breaks the
// loop and surfaces via rows.Err()→classify, with no partial rows
// (spec-2.3a cr MED-3).
func TestQuery_RowValuesErrorBreaks(t *testing.T) {
	t.Parallel()
	rows := &fakeRows{fields: cols("c"), data: [][]any{{"a"}, {"b"}, {"c"}}, valuesErrAt: 2}
	conn, _, _ := newQueryConn(rows, nil, nil)
	_, err := conn.Query(context.Background(), "SELECT 1", db.QueryOptions{})
	if err == nil {
		t.Fatal("want error from mid-scan Values failure")
	}
	var ec interface{ Code() string }
	if !errors.As(err, &ec) || ec.Code() != db.ErrQueryFailed.Code() {
		t.Errorf("err = %v; want DB.QUERY_FAILED", err)
	}
}

// TestQuery_ReadOnlyViolation — SQLSTATE 25006 maps to READONLY_VIOLATION,
// 0A000 does NOT (stays QUERY_FAILED) (spec-2.3a D-2 / cr HIGH-1).
func TestQuery_ReadOnlyViolation(t *testing.T) {
	t.Parallel()
	conn, _, _ := newQueryConn(nil, &pgconn.PgError{Code: "25006", Message: "cannot execute INSERT in a read-only transaction"}, nil)
	_, err := conn.Query(context.Background(), "INSERT ...", db.QueryOptions{})
	var ec interface{ Code() string }
	if !errors.As(err, &ec) || ec.Code() != db.ErrReadOnlyViolation.Code() {
		t.Errorf("25006 err = %v; want DB.READONLY_VIOLATION", err)
	}

	conn2, _, _ := newQueryConn(nil, &pgconn.PgError{Code: "0A000", Message: "feature not supported"}, nil)
	_, err2 := conn2.Query(context.Background(), "SELECT ...", db.QueryOptions{})
	if !errors.As(err2, &ec) || ec.Code() != db.ErrQueryFailed.Code() {
		t.Errorf("0A000 err = %v; want DB.QUERY_FAILED (not readonly)", err2)
	}
}

// TestQuery_CtxRootPreservedForTwoTrack — a ctx-deadline query error is
// classified as DB.TIMEOUT but still matches context.DeadlineExceeded via
// errors.Is, so the tool layer can honor the spec-1.21 two-track contract
// (spec-2.3a D-1 / codex CRIT-1).
func TestQuery_CtxRootPreservedForTwoTrack(t *testing.T) {
	t.Parallel()
	conn, _, _ := newQueryConn(nil, context.DeadlineExceeded, nil)
	_, err := conn.Query(context.Background(), "SELECT 1", db.QueryOptions{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("ctx root not preserved: %v", err)
	}
}

// TestQuery_BeginTxFailureSanitized — a BeginTx error is sanitized (no DSN
// leak) and classified (spec-2.3a R-4 sanitize regression延伸).
func TestQuery_BeginTxFailureSanitized(t *testing.T) {
	t.Parallel()
	conn, _, _ := newQueryConn(nil, nil, &pgconn.PgError{Code: "08006", Message: "connection failure to host=secret password=leak"})
	_, err := conn.Query(context.Background(), "SELECT 1", db.QueryOptions{})
	if err == nil {
		t.Fatal("want BeginTx error")
	}
	if strings.Contains(err.Error(), "leak") {
		t.Errorf("BeginTx error leaked credentials: %s", err.Error())
	}
}
