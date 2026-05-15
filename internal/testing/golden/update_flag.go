// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package golden

import "flag"

// init registers -update once per binary unless another package has
// already registered it. We do NOT hold a *bool pointer — the flag
// value is read live via flag.Getter on each Update() call (spec-0.11
// R3 codex round-3 HIGH-1: prior *bool reuse design could not read
// post-parse updates from foreign-registered flags).
//
//nolint:gochecknoinits // spec-0.11 D-3: flag registration is intentionally a side effect.
func init() {
	if flag.Lookup("update") != nil {
		return // pre-existing -update flag; will read live via Update()
	}
	flag.Bool("update", false, "update golden files on mismatch")
}

// Update reports whether the -update flag is set. Reads live so we
// work whether the flag was registered by us or by an importing test
// binary; required for `go test -update` parse-order independence.
//
// If -update is not registered at all, returns false (default off).
// If -update is registered as non-bool (semantically incompatible),
// panics — intentional, because mixing flag families is a bug.
func Update() bool {
	f := flag.Lookup("update")
	if f == nil {
		return false
	}
	g, ok := f.Value.(flag.Getter)
	if !ok {
		return false // pre-Go-1.2 non-Getter Value; treat as off
	}
	b, ok := g.Get().(bool)
	if !ok {
		panic("internal/testing/golden: -update flag is registered but not bool")
	}
	return b
}
