// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File errors.go — db_query input-validation errcode (spec-2.3a D-5).
// Chinese message/hint to match the existing DB.* convention in
// domain/db/errors.go (spec-2.3a Q11 user decision — same DB package
// family, do not mix languages within it).
//
// This is the only NEW code owned by this package; the read/connect/
// timeout/readonly codes all live in domain/db (registered by spec-1.18
// and spec-2.3a D-5) and are reused via the sanitized classify path.

package dbquery

import "github.com/sqlrush/opendbx/internal/platform/errcode"

//nolint:gochecknoglobals // spec-0.6 contract: errcode sentinels are package-level.
var (
	// ErrQueryInputInvalid — the db_query tool input is malformed (missing /
	// non-string / blank "sql"). Feedback class: the composed Content is
	// written into a recoverable ToolResult so the model self-corrects.
	ErrQueryInputInvalid = errcode.Register(
		"DB.QUERY_INPUT_INVALID",
		"db_query 入参无效",
		`用 {"sql": "<一条只读语句>"} 调用 db_query`,
	)
)

// inputInvalidContent composes the recoverable ToolOutput.Content for a
// malformed db_query input (spec-2.3a D-5). detail is the fixed-variant
// reason (missing/non-string skill field etc.). Mirrors the spec-2.3
// invoke template shape: "[CODE] message: detail. Hint: hint."
func inputInvalidContent(detail string) string {
	return "[" + ErrQueryInputInvalid.Code() + "] " + ErrQueryInputInvalid.Message() +
		": " + detail + ". Hint: " + ErrQueryInputInvalid.Hint() + "."
}
