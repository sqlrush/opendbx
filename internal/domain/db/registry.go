// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File registry.go — process-level driver registry (spec-1.18 D-1). Mirrors
// database/sql.Register / errcode.Register conventions: drivers self-register
// in init; duplicate registration is a programmer error and panics; the map
// is guarded by an RWMutex because Open (read) runs concurrently with the
// diagnosis agent while drivers register once at init.
//
// Design: spec-1.18-pg-driver.

package db

import (
	"context"
	"sort"
	"sync"

	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

//nolint:gochecknoglobals // spec-1.18 D-1: process-level driver registry, mirrors database/sql.
var (
	mu      sync.RWMutex
	drivers = map[string]Driver{}
)

// Register makes a Driver available by its Name. Intended to be called from
// a driver package's init. A duplicate name panics (init-time programmer
// error — there is no error return to handle from init, mirroring
// database/sql.Register and errcode.Register).
func Register(d Driver) {
	mu.Lock()
	defer mu.Unlock()
	name := d.Name()
	if _, dup := drivers[name]; dup {
		panic("db: duplicate driver registration: " + name)
	}
	drivers[name] = d
}

// Open looks up a registered driver by name and opens a connection. An
// unknown driver name returns DB.DRIVER_UNKNOWN (runtime error with a Hint),
// not a panic — the name may come from user config.
func Open(ctx context.Context, driverName, dsn string) (Conn, error) {
	mu.RLock()
	d, ok := drivers[driverName]
	mu.RUnlock()
	if !ok {
		return nil, errcode.Newf(ErrDriverUnknown.Code(), "未注册的数据库 driver: %s", driverName)
	}
	// errcode-lint:exempt -- spec-1.18 D-1: delegates to Driver.Open, whose error is already a sanitized errcode.Error (postgres classify); registry is a pass-through, not the error origin.
	return d.Open(ctx, dsn)
}

// registeredNames returns the sorted set of registered driver names. Used by
// tests; kept unexported (production code does not enumerate drivers).
func registeredNames() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(drivers))
	for name := range drivers {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// unregisterForTesting removes a driver registration. Test-only helper (not
// exported — production never deregisters) so tests can Register + Cleanup and
// stay idempotent across `go test -count>1` (spec-1.18 R-fix; post-impl
// go-reviewer HIGH-1). Mirrors errcode.unregisterForTesting.
func unregisterForTesting(name string) {
	mu.Lock()
	defer mu.Unlock()
	delete(drivers, name)
}
