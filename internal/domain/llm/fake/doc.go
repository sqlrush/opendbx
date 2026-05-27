// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package fake is a scripted llm.Provider for integration / E2E tests
// (spec-1.20 D-3; N-A deferred item landed here). Zero vendor SDK; peer
// to anthropic/ as a Provider implementation so cross-package tests can
// import it (Go _test.go is not cross-package).
//
// spec-1.20 ships a single-turn scripted provider. spec-1.21 multi-turn
// tool loop will add ScriptedTurns([]turn) (per-request script) as an
// additive extension — the spec-1.20 New/Scripted signatures do not break.
package fake
