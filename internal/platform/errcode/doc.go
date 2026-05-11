// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush
//
// Package errcode implements opendbx's structured error contract
// (CLAUDE.md 规则 7, derived from pgrac 17 + opendb E4).
//
// Every public/API-boundary error returned from opendbx packages MUST carry
// three pieces of information:
//
//   - Code:    machine-readable identifier, e.g. "LOGGER.WRITER_CLOSED".
//     Format: MODULE.NOUN_VERB (uppercase, dotted prefix).
//   - Message: single human-readable sentence describing what happened.
//   - Hint:    1-2 sentences telling the user how to recover.
//
// The Error interface (errcode.go) plus the central registry (registry.go)
// give callers a uniform way to construct, match, and forward errors. The
// sidecar JSONL stream (spec-0.5 § 2.3) consumes errcode.Error to populate
// its `error: {code, message, hint}` field; that path uses errors.As so
// wrapped chains (fmt.Errorf("%w"), errcode.Wrap, redactedError) all
// surface the structured Code rather than degrading to plain text.
//
// Design: spec-0.6-error-codes.md (FROZEN <tag>).
package errcode
