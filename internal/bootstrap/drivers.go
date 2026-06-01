// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File drivers.go — production database-driver registration (spec-1.19 R-fix;
// post-impl T-10a codex HIGH-1).
//
// db.Driver implementations self-register in their package init(), so the
// driver package must be imported for that init to run. bootstrap is the
// wiring layer (it already imports both platform/config and domain/db), so it
// owns the side-effect imports for every supported driver. Without this, a
// production `driver: postgres` config resolves to DB.DRIVER_UNKNOWN because
// nothing in the cmd/opendbx → bootstrap → domain/db dependency graph imports
// the postgres package (the only importer was a test-only blank import, which
// masked the gap). Adding a new driver (mysql/oracle/openGauss in Stage 6+) is
// one more blank import line here.

package bootstrap

import (
	// Register the postgres driver (db.Register in its init) so db.Open /
	// ResolveDSN can resolve `driver: postgres` in the production binary.
	_ "github.com/sqlrush/opendbx/internal/domain/db/postgres"
)
