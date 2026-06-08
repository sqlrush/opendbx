// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package dbquery

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/sqlrush/opendbx/internal/domain/db"
)

// --- fakes ---

// fakeQueryConn implements db.QueryConn.
type fakeQueryConn struct {
	result   db.QueryResult
	queryErr error
	closed   int
}

func (f *fakeQueryConn) Ping(context.Context) error { return nil }
func (f *fakeQueryConn) HealthCheck(context.Context) (db.Health, error) {
	return db.Health{}, nil
}
func (f *fakeQueryConn) Close() error { f.closed++; return nil }
func (f *fakeQueryConn) Query(_ context.Context, _ string, _ db.QueryOptions) (db.QueryResult, error) {
	return f.result, f.queryErr
}

// fakeConn implements db.Conn only (NOT QueryConn) — for the capability
// assertion-failure path.
type fakeConn struct{ closed int }

func (f *fakeConn) Ping(context.Context) error                     { return nil }
func (f *fakeConn) HealthCheck(context.Context) (db.Health, error) { return db.Health{}, nil }
func (f *fakeConn) Close() error                                   { f.closed++; return nil }

// --- Schema / Name ---

func TestTool_NameAndSchema(t *testing.T) {
	t.Parallel()
	tool := New(func(context.Context) (db.Conn, error) { return &fakeQueryConn{}, nil })
	if tool.Name() != "db_query" {
		t.Errorf("Name = %q", tool.Name())
	}
	sch := tool.Schema()
	if sch.Name != tool.Name() {
		t.Errorf("Schema.Name %q must equal Name()", sch.Name)
	}
	req, _ := sch.InputSchema["required"].([]string)
	if len(req) != 1 || req[0] != "sql" {
		t.Errorf("required = %v; want [sql]", req)
	}
}

// --- input validation ---

func TestTool_InputInvalid(t *testing.T) {
	t.Parallel()
	tool := New(func(context.Context) (db.Conn, error) { return &fakeQueryConn{}, nil })
	for _, in := range []map[string]any{
		{},             // missing
		{"sql": 42},    // non-string
		{"sql": "   "}, // blank
	} {
		out, err := tool.Execute(context.Background(), in)
		if err != nil {
			t.Fatalf("semantic failure must not return Go error: %v", err)
		}
		if !out.IsError || !strings.Contains(out.Content, "DB.QUERY_INPUT_INVALID") {
			t.Errorf("input %v → %+v; want IsError INPUT_INVALID", in, out)
		}
	}
}

// --- ctx two-track (codex CRIT-1) ---

func TestTool_CtxFatalAtEntry(t *testing.T) {
	t.Parallel()
	tool := New(func(context.Context) (db.Conn, error) { return &fakeQueryConn{}, nil })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out, err := tool.Execute(ctx, map[string]any{"sql": "SELECT 1"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled ctx at entry → err %v; want context.Canceled", err)
	}
	if out.IsError || out.Content != "" {
		t.Errorf("ctx fatal must not yield recoverable output: %+v", out)
	}
}

func TestTool_CtxFatalOnOpen(t *testing.T) {
	t.Parallel()
	// openFn returns a ctx-wrapped error (as bootstrap's OpenConnection would
	// under a deadline) — must be fatal, not a recoverable DB.TIMEOUT result.
	tool := New(func(context.Context) (db.Conn, error) {
		return nil, context.DeadlineExceeded
	})
	out, err := tool.Execute(context.Background(), map[string]any{"sql": "SELECT 1"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("open ctx error → %v; want DeadlineExceeded fatal", err)
	}
	if out.IsError {
		t.Errorf("must not be recoverable: %+v", out)
	}
}

func TestTool_CtxFatalOnQuery(t *testing.T) {
	t.Parallel()
	qc := &fakeQueryConn{queryErr: context.DeadlineExceeded}
	tool := New(func(context.Context) (db.Conn, error) { return qc, nil })
	out, err := tool.Execute(context.Background(), map[string]any{"sql": "SELECT 1"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("query ctx error → %v; want DeadlineExceeded fatal", err)
	}
	if out.IsError {
		t.Errorf("must not be recoverable: %+v", out)
	}
}

// --- capability assertion failure closes the conn (codex HIGH-2) ---

func TestTool_CapabilityFailureClosesConn(t *testing.T) {
	t.Parallel()
	fc := &fakeConn{}
	tool := New(func(context.Context) (db.Conn, error) { return fc, nil })
	out, err := tool.Execute(context.Background(), map[string]any{"sql": "SELECT 1"})
	if err != nil {
		t.Fatalf("capability failure is recoverable, not Go error: %v", err)
	}
	if !out.IsError || !strings.Contains(out.Content, "不支持查询") {
		t.Errorf("want recoverable 'driver does not support queries': %+v", out)
	}
	if fc.closed != 1 {
		t.Errorf("non-query Conn must be Closed exactly once, got %d", fc.closed)
	}
}

// --- lazy open: failure not cached, retryable (Q4) ---

func TestTool_LazyOpenRetryable(t *testing.T) {
	t.Parallel()
	var calls int
	qc := &fakeQueryConn{result: db.QueryResult{Columns: []string{"c"}, Rows: [][]string{{"1"}}}}
	tool := New(func(context.Context) (db.Conn, error) {
		calls++
		if calls == 1 {
			return nil, db.ErrConnectFailed // first open fails
		}
		return qc, nil
	})
	// First Execute: open fails → recoverable, not cached.
	out1, err := tool.Execute(context.Background(), map[string]any{"sql": "SELECT 1"})
	if err != nil || !out1.IsError {
		t.Fatalf("first call should be recoverable failure: out=%+v err=%v", out1, err)
	}
	// Second Execute: retried open succeeds.
	out2, err := tool.Execute(context.Background(), map[string]any{"sql": "SELECT 1"})
	if err != nil || out2.IsError {
		t.Fatalf("second call should succeed (retry): out=%+v err=%v", out2, err)
	}
	if calls != 2 {
		t.Errorf("openFn calls = %d; want 2 (first not cached)", calls)
	}
	// Third Execute: cached conn reused, no new open.
	if _, err := tool.Execute(context.Background(), map[string]any{"sql": "SELECT 2"}); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Errorf("openFn calls after cache = %d; want still 2", calls)
	}
}

// --- readonly violation surfaces the read-only hint ---

func TestTool_ReadOnlyViolationContent(t *testing.T) {
	t.Parallel()
	qc := &fakeQueryConn{queryErr: db.ErrReadOnlyViolation}
	tool := New(func(context.Context) (db.Conn, error) { return qc, nil })
	out, err := tool.Execute(context.Background(), map[string]any{"sql": "INSERT ..."})
	if err != nil || !out.IsError {
		t.Fatalf("readonly violation is recoverable: out=%+v err=%v", out, err)
	}
	if !strings.Contains(out.Content, "DB.READONLY_VIOLATION") || !strings.Contains(out.Content, "只读") {
		t.Errorf("content must carry readonly code + hint: %q", out.Content)
	}
}

// --- success renders a table ---

func TestTool_SuccessRendersTable(t *testing.T) {
	t.Parallel()
	qc := &fakeQueryConn{result: db.QueryResult{
		Columns: []string{"id", "name"},
		Rows:    [][]string{{"1", "alice"}, {"2", "bob"}},
	}}
	tool := New(func(context.Context) (db.Conn, error) { return qc, nil })
	out, err := tool.Execute(context.Background(), map[string]any{"sql": "SELECT id,name FROM t"})
	if err != nil || out.IsError {
		t.Fatalf("success: out=%+v err=%v", out, err)
	}
	for _, want := range []string{"id", "name", "alice", "(2 rows)"} {
		if !strings.Contains(out.Content, want) {
			t.Errorf("table missing %q:\n%s", want, out.Content)
		}
	}
}

// --- concurrency: mu guards conn (defense-in-depth) ---

func TestTool_ConcurrentExecuteRace(t *testing.T) {
	t.Parallel()
	qc := &fakeQueryConn{result: db.QueryResult{Columns: []string{"c"}, Rows: [][]string{{"x"}}}}
	tool := New(func(context.Context) (db.Conn, error) { return qc, nil })
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = tool.Execute(context.Background(), map[string]any{"sql": "SELECT 1"})
		}()
	}
	wg.Wait()
}
