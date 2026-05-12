// Module: pre-push-hook-test
//
// Go test wrapper that exec-s git-hooks/pre-push under fixture-controlled
// git repos to assert the 5 invariants from spec-0.7 § 2.5.
//
// Stdlib only (os/exec, testing); each test builds a throwaway git repo
// in t.TempDir() so production state stays untouched.
//
// Author: sqlrush
// Design: opendbx spec-0.7-version-numbering.md § 2.5 D-5 / T-8

module github.com/sqlrush/opendbx/git-hooks/test

go 1.22
