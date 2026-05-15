// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package must provides assertion helpers that fail the test fast when
// preconditions don't hold. Consolidates the ad-hoc must-style helpers
// (mustParse / mustCheck / mustGit / writeFile etc.) currently copied
// across tools/ test files.
//
// All helpers:
//   - Take testing.TB so they work with *testing.T, *testing.B, and
//     mock TBs for negative-path testing.
//   - Call t.Helper() so failures point at the caller, not the helper.
//   - Use t.Fatalf on failure (test halts immediately).
//   - Are binary-safe where applicable (no string coercion of payloads).
//
// Design: spec-0.11-test-framework § 1.1 D-2.
package must
