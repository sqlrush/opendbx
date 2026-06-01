// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/sqlrush/opendbx/internal/domain/db"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want error // db.Err* sentinel, or nil
	}{
		{"nil", nil, nil},
		{"ctx deadline", context.DeadlineExceeded, db.ErrTimeout},
		{"ctx cancelled", context.Canceled, db.ErrTimeout},
		{"wrapped ctx deadline", fmt.Errorf("dial: %w", context.DeadlineExceeded), db.ErrTimeout},
		{"auth 28P01", &pgconn.PgError{Code: "28P01"}, db.ErrAuthFailed},
		{"auth 28000", &pgconn.PgError{Code: "28000"}, db.ErrAuthFailed},
		{"insufficient_privilege 42501", &pgconn.PgError{Code: "42501"}, db.ErrAuthFailed},
		{"connect 08006", &pgconn.PgError{Code: "08006"}, db.ErrConnectFailed},
		{"invalid_catalog 3D000", &pgconn.PgError{Code: "3D000"}, db.ErrConnectFailed},
		{"resources 53300", &pgconn.PgError{Code: "53300"}, db.ErrUnavailable},
		{"operator 57P03", &pgconn.PgError{Code: "57P03"}, db.ErrUnavailable},
		{"syntax 42601", &pgconn.PgError{Code: "42601"}, db.ErrQueryFailed},
		{"integrity 23505", &pgconn.PgError{Code: "23505"}, db.ErrQueryFailed},
		{"malformed empty code", &pgconn.PgError{Code: ""}, db.ErrQueryFailed},
		{"malformed short code", &pgconn.PgError{Code: "0"}, db.ErrQueryFailed},
		{"non-pgerror", errors.New("dial tcp: connection refused"), db.ErrConnectFailed},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := classify(tc.err)
			if tc.want == nil {
				if got != nil {
					t.Fatalf("classify(%v) = %v, want nil", tc.err, got)
				}
				return
			}
			if !errors.Is(got, tc.want) {
				t.Fatalf("classify(%v): want errors.Is %v, got %v", tc.err, tc.want, got)
			}
		})
	}
}

// TestClassifyPreservesRoot asserts the sanitized error still matches the pgx
// root via errors.As / errors.Is, while the code matches via the sentinel.
func TestClassifyPreservesRoot(t *testing.T) {
	root := &pgconn.PgError{Code: "28P01", Message: "password authentication failed"}
	got := classify(root)

	if !errors.Is(got, db.ErrAuthFailed) {
		t.Error("lost db.ErrAuthFailed code match")
	}
	var pgErr *pgconn.PgError
	if !errors.As(got, &pgErr) {
		t.Fatal("lost *pgconn.PgError via errors.As")
	}
	if pgErr.Code != "28P01" {
		t.Errorf("root PgError.Code = %q, want 28P01", pgErr.Code)
	}

	// ctx root preserved too.
	ctxErr := classify(context.DeadlineExceeded)
	if !errors.Is(ctxErr, context.DeadlineExceeded) {
		t.Error("lost context.DeadlineExceeded match")
	}
	if !errors.Is(ctxErr, db.ErrTimeout) {
		t.Error("lost db.ErrTimeout code match")
	}
}

// TestClassifyNoDSNLeak is the security-critical test (spec-1.18 R-3, three-
// route review HIGH): a connection-phase error whose text embeds the DSN
// (host / user / password) must NOT appear in the classified error's Error()
// text, while the root is still preserved for matching.
func TestClassifyNoDSNLeak(t *testing.T) {
	const (
		password = "sup3rs3cr3t"
		host     = "db.internal.corp"
		user     = "admin"
	)
	// Mimics a pgxpool.New parse/dial error that embeds the connection string.
	leaky := fmt.Errorf(
		"failed to connect to `host=%s user=%s database=prod`: password=%s rejected",
		host, user, password,
	)
	got := classify(leaky)

	rendered := got.Error()
	for _, secret := range []string{password, host, user, "password="} {
		if strings.Contains(rendered, secret) {
			t.Errorf("classified Error() leaked %q: %s", secret, rendered)
		}
	}
	// It is still a connect failure and still preserves the root for debugging.
	if !errors.Is(got, db.ErrConnectFailed) {
		t.Error("want db.ErrConnectFailed")
	}
	if !errors.Is(got, leaky) {
		t.Error("root error not preserved in Unwrap chain")
	}
}

// TestSanitizedErrorRendersCodeOnly checks the safeErr Error() shape.
func TestSanitizedErrorRendersCodeOnly(t *testing.T) {
	got := classify(&pgconn.PgError{Code: "08006", Message: "server closed the connection"})
	if !strings.Contains(got.Error(), "DB.CONNECT_FAILED") {
		t.Errorf("Error() should render the DB code, got %q", got.Error())
	}
	if strings.Contains(got.Error(), "server closed the connection") {
		t.Errorf("Error() leaked pgx message text: %q", got.Error())
	}
}
