// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File tool.go — the db_query ToolExecutor (spec-2.3a D-3).
//
// Two-track failure (spec-1.21 contract, codex CRIT-1): ctx cancel /
// deadline is FATAL — returned as a Go error so the Loop's
// classifyToolErr produces FinishCancelled / DIAGNOSE.TOOL_TIMEOUT rather
// than hiding it behind a recoverable [DB.TIMEOUT] tool result. Every
// open/query error is checked for a ctx root FIRST (the postgres classify
// preserves it under Unwrap). Only non-ctx failures become recoverable
// IsError tool results the model can self-correct on.

package dbquery

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/sqlrush/opendbx/internal/app/diagnose"
	"github.com/sqlrush/opendbx/internal/domain/db"
	"github.com/sqlrush/opendbx/internal/domain/llm"
	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

// ToolName is the wire-visible tool identifier — lowercase opendbx-native
// (clock/echo family; the CC-parity capitalization rule applies only to
// CC tools, and DB query has no CC counterpart — spec-2.3a Q3).
const ToolName = "db_query"

// errDriverNoQuery is an internal sentinel: the opened db.Conn does not
// implement db.QueryConn (a future non-query driver). Never escapes
// Execute — it is converted to a recoverable tool result.
var errDriverNoQuery = errors.New("driver does not support read-only queries")

// Tool is the db_query ToolExecutor. The connection is opened lazily on
// first Execute (startup stays DB-I/O-free, spec-2.3a Q4) and cached.
type Tool struct {
	openFn func(ctx context.Context) (db.Conn, error)
	mu     sync.Mutex // defense-in-depth only: Loop is serial and the
	// registry is built per-session, so there is no
	// concurrent Execute today (spec-2.3a arch HIGH-1).
	conn db.QueryConn // cached after first successful open+assert; nil until then
}

// Compile-time: *Tool is a diagnose-loop-callable tool.
var _ diagnose.ToolExecutor = (*Tool)(nil)

// New builds the tool with a lazy connection factory. bootstrap wraps
// OpenConnection in openFn so this package imports neither config nor
// bootstrap (layer hygiene).
func New(openFn func(ctx context.Context) (db.Conn, error)) *Tool {
	return &Tool{openFn: openFn}
}

// Name implements diagnose.ToolExecutor.
func (t *Tool) Name() string { return ToolName }

// Schema implements diagnose.ToolExecutor. Decision-tree description
// (CLAUDE.md § 3.3): when to use / when NOT to use, not a usage manual.
func (t *Tool) Schema() llm.ToolSchema {
	return llm.ToolSchema{
		Name: ToolName,
		Description: "Run ONE read-only SQL statement against the active PostgreSQL " +
			"connection (SELECT / WITH / EXPLAIN / SHOW). Use to inspect live database " +
			"state — pg_stat_* views, catalog queries, plans. Do NOT use for writes: " +
			"INSERT/UPDATE/DELETE/DDL are rejected server-side. Results are capped at " +
			"100 rows and 16KiB.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"sql": map[string]any{
					"type":        "string",
					"description": "One read-only SQL statement.",
				},
			},
			"required": []string{"sql"},
		},
	}
}

// Execute implements diagnose.ToolExecutor (spec-2.3a D-3 steps 1-5).
func (t *Tool) Execute(ctx context.Context, input map[string]any) (diagnose.ToolOutput, error) {
	// errcode-lint:exempt -- spec-1.21 D-4 two-track: ctx errors pass through unchanged; the Loop classifies cancel-vs-timeout.
	if err := ctx.Err(); err != nil {
		return diagnose.ToolOutput{}, err
	}
	sql, ok := input["sql"].(string)
	if !ok || strings.TrimSpace(sql) == "" {
		return diagnose.ToolOutput{
			Content: inputInvalidContent("缺少或非字符串 sql 字段"),
			IsError: true,
		}, nil
	}

	qc, err := t.connection(ctx)
	if err != nil {
		if isCtxErr(ctx, err) {
			// errcode-lint:exempt -- spec-2.3a D-3: ctx fatal precedes the DB.* recoverable path (codex CRIT-1).
			return diagnose.ToolOutput{}, err
		}
		if errors.Is(err, errDriverNoQuery) {
			return diagnose.ToolOutput{
				Content: "[" + db.ErrQueryFailed.Code() + "] 当前数据库 driver 不支持查询. Hint: 确认 driver 实现 QueryConn 能力",
				IsError: true,
			}, nil
		}
		return diagnose.ToolOutput{Content: dbErrContent(err), IsError: true}, nil
	}

	res, err := qc.Query(ctx, sql, db.QueryOptions{})
	if err != nil {
		if isCtxErr(ctx, err) {
			// errcode-lint:exempt -- spec-2.3a D-3: ctx fatal precedes the DB.* recoverable path.
			return diagnose.ToolOutput{}, err
		}
		return diagnose.ToolOutput{Content: dbErrContent(err), IsError: true}, nil
	}
	return diagnose.ToolOutput{Content: renderTable(res)}, nil
}

// connection returns the cached QueryConn, opening it lazily on first use.
// A failed open is NOT cached (retryable on the next Execute — DB recovery
// needs no restart, spec-2.3a Q4). On a successful open whose Conn lacks
// the query capability, the Conn is Closed before erroring (no pool leak,
// codex HIGH-2).
func (t *Tool) connection(ctx context.Context) (db.QueryConn, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.conn != nil {
		return t.conn, nil
	}
	c, err := t.openFn(ctx)
	if err != nil {
		return nil, err // not cached → retryable
	}
	qc, ok := c.(db.QueryConn)
	if !ok {
		_ = c.Close() // prevent pool leak (codex HIGH-2)
		return nil, errDriverNoQuery
	}
	t.conn = qc
	return qc, nil
}

// isCtxErr reports whether err (or the live ctx) is a cancel/deadline that
// must terminate as a Go error rather than a recoverable tool result. The
// postgres classify wraps ctx into DB.TIMEOUT but preserves the root under
// Unwrap, so errors.Is still matches (spec-2.3a codex CRIT-1).
func isCtxErr(ctx context.Context, err error) bool {
	return ctx.Err() != nil ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded)
}

// dbErrContent renders a sanitized DB.* error to recoverable tool-result
// text. The classify path guarantees errcode.Error with no DSN leak; the
// hint carries the actionable guidance (read-only violations already say
// "写操作被服务端拒绝" in their hint, spec-2.3a D-5).
func dbErrContent(err error) string {
	var ec errcode.Error
	if errors.As(err, &ec) {
		if h := ec.Hint(); h != "" {
			return ec.Error() + ". Hint: " + h
		}
		return ec.Error()
	}
	// Unreachable: classify always returns errcode.Error. Defensive only.
	return "[" + db.ErrQueryFailed.Code() + "] " + err.Error()
}
