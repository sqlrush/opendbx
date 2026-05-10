# tools/import-rules-check

Self-contained Go binary that enforces opendbx's layered import-direction
rules per **spec-0.2 § 2.2** + **spec-0.3 § 2.2** + **spec-0.4 § 2.2**.

## Usage

```bash
# from opendbx repo root:
go run ./tools/import-rules-check -v .
# exits 0 on clean, 1 on violation, 2 on fatal scan error
```

CI invokes this in the **Import Rules Check** job (spec-0.9 ci.yml).

## Three rule families

1. **Layer matrix** — `cmd → entrypoints → bootstrap → app/domain/platform`
   strict downward chain. The *only* `cmd → platform` exception is
   `internal/platform/version` (linker-injected version string read by
   `--version`). All other platform/* imports must route through
   `internal/entrypoints/*` relays.
2. **Cluster restrictions** — services mutual ban (`services/<A>` ↛
   `services/<B>`), DB driver isolation (`db/postgres` ↛ `db/mysql`),
   render subsystem boundary (`scrollback` ↛ `cli/components`).
3. **Render strict DAG** — `terminal → buffer → layout → optimizer →
   scheduler → scrollback → streaming → block → style/width`. Reverse
   import in any direction fails.

## Plus filesystem checks

- Every `internal/<...>/` directory containing a `.go` file MUST have
  a `doc.go` (per CLAUDE.md package convention).
- `pkg/` MUST stay empty in spec-0.2 era (no `.go` files allowed).

## Test fixture style

`main_test.go` uses 50+ table-driven cases covering layer matrix +
cluster + render DAG + filesystem. Run with `go test -race ./tools/...`
in repo root.

## Triggered violations

If you intentionally want to verify the tool catches a class of
violations, e.g.:

```bash
# fake a cmd → platform/config violation (forbidden):
echo 'package main; import _ "github.com/sqlrush/opendbx/internal/platform/config"' > /tmp/v.go
# tool will exit 1 and explain
```

## Why self-built (not depguard / goda)

spec-0.2 Q4 ★A decision: opendbx-specific rules (cluster restrictions,
render DAG, single-exception list) are not expressible in third-party
linters. Self-built ~750 LOC + tests gives full control + zero
external lint dependencies. Future re-evaluation in spec-0.10.

## Related specs

- spec-0.2 § 2.2 (layer matrix definition)
- spec-0.3 § 2.2 (cmd → platform/version exception confirmation)
- spec-0.4 (config layer added; routes through entrypoints to keep
  exception list at 1)
- spec-0.10 (broader lint static analysis; may extend this tool's
  rule set)
