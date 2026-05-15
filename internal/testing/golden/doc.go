// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package golden standardises golden file testing across opendbx:
//   - Compare(t, name, got)         — default-path golden, binary-safe
//   - CompareString(t, name, got)   — UTF-8 text convenience
//   - CompareFile(t, relPath, got)  — explicit relative path overload
//     for existing corpora like cmd/opendbx/testdata/golden/*.txt that
//     share files across tests and use non-.golden suffixes.
//   - Update() bool                 — reports current -update flag state
//
// `-update` flag semantics
//
// The package registers a global -update bool flag at init() unless
// another package has already done so. Value is read LIVE via
// flag.Getter on each Compare call (NOT cached as a *bool pointer)
// because we don't own the storage when -update was registered
// elsewhere. See R3 codex round-3 HIGH-1.
//
// Safe usage:
//   go test -update ./internal/testing/golden    # package-level: safe
//   go test -update ./...                        # MAY fail if some
//                                                  packages don't link
//                                                  this package
//
// Design: spec-0.11-test-framework § 1.1 D-3.
package golden
