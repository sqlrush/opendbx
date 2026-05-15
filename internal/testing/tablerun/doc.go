// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package tablerun standardises table-driven test boilerplate across
// opendbx. It provides:
//
//   - Run[T] — serial execution of test cases (default; safe for tests
//     that use t.Setenv / os.Chdir / global state).
//   - RunParallel[T] — opt-in t.Parallel execution.
//   - Skippable interface — optional per-case skip via SkipReason().
//   - mustExtractName — reflective lookup of the required Name string
//     field; fails the test fast if Name is missing, non-string, or
//     empty.
//
// Design: spec-0.11-test-framework § 1.1 D-1.
//
// Why default serial? See spec-0.11 R2 codex HIGH-3: opendbx has many
// existing _test.go that mutate process-global state (t.Setenv, global
// loggers, version vars). A blind parallel default would panic
// (t.Setenv under parallel ancestor) or race. RunParallel exists for
// fully audited corpora.
package tablerun
