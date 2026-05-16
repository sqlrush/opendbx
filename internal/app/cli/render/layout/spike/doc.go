// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

//go:build spike
// +build spike

// Package spike is a THROW-AWAY prototype implementing a Yoga-like flex
// subset (4 dimensions: direction / grow / shrink / basis) used to
// validate the AD-002 risk hypothesis that opendbx can self-build a
// usable flex layout engine for Stage 1 spec-1.1-flex-layout.
//
// **DO NOT IMPORT FROM PRODUCTION CODE.** This package is excluded from
// the default `go build ./...` via the `//go:build spike` build tag.
// To run spike tests:
//
//	go test -tags=spike ./internal/app/cli/render/layout/spike/...
//	go test -tags=spike -run=^$ -bench=. ./internal/app/cli/render/layout/spike/...
//
// Outcome of the spike (A self-built / B yoga-go fallback / C partial)
// is recorded in opendbrb/docs/spikes/spec-0.12.5-flex-spike-report.md.
// spec-1.1-flex-layout consumes that report and decides production path.
//
// Design: opendbrb/specs/stage-0/spec-0.12.5-flex-spike.md § 1.3 (D-1)
package spike
