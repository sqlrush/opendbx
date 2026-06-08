// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package dbquery provides the db_query diagnose ToolExecutor (spec-2.3a):
// a read-only SQL entry the LLM can call against the active PostgreSQL
// connection. It is the tool substrate the bundled DB diagnostic skills
// (spec-2.3b) drive via `allowed-tools: db_query`.
//
// Read-only is enforced SERVER-side (the db.QueryConn driver runs every
// statement inside a READ ONLY transaction); this package never inspects
// SQL text. Failures are two-track per the spec-1.21 ToolExecutor
// contract: ctx cancel/deadline → fatal Go error; semantic failures
// (bad input, query error, read-only violation) → recoverable ToolOutput
// with IsError so the model self-corrects.
//
// Layer hygiene: this package imports db / diagnose / llm / errcode and
// stdlib only. The connection factory is injected as a closure by
// bootstrap (openFn) so dbquery imports neither config nor bootstrap.
package dbquery
