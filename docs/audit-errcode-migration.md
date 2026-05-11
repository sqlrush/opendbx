# spec-0.6 D-4 audit — `fmt.Errorf` migration classification

> Spec: `opendbrb/specs/stage-0/spec-0.6-error-codes.md` § 1.3.1 boundary
> 6-class definitions + Q11 B+ scope.
> Auditor: sqlrush + claude code-reviewer (HIGH-1 grep verified).
> Scan command:
> `grep -rnE 'fmt\.Errorf\("[^%]' internal/ cmd/ --include="*.go" | grep -v "_test.go"`
> Total `fmt.Errorf` sites (inline new error, not `%w`-wrapping): **40**.
> (Spec R1 estimated 10-15; claude HIGH-1 corrected to 25; final scan finds 40.)

## Classification key

| Class | Definition | Action |
|---|---|---|
| **boundary** | Exported API returning to cmd/opendbx or entrypoints relay; user-visible config / admin / logger sidecar surface | **Migrate to `errcode.Newf` / `errcode.Wrap`** (spec-0.6 T-8 scope) |
| **private** | Internal helper inside a single package; never crosses boundary directly | Keep `fmt.Errorf` if upper boundary wraps with `errcode.Wrap` |
| **external** | Wraps stdlib / 3rd-party root (e.g. `os.ErrNotExist`, `yaml.Decode`) where the root carries enough context; the public surface should add the structured code at its layer | Keep `fmt.Errorf("%w", root)`; rely on outer boundary to `errcode.Wrap` |
| **already-wrapped** | Site is already inside an `errcode.Wrap` or returns through a function that wraps; no action needed | none |

## Sites by file

### `internal/platform/config/env.go` (14 sites)

| Line region | Pattern | Class | Decision |
|---|---|---|---|
| ENV parse errors (uint8/int/duration/bool/oneof) | `fmt.Errorf("OPENDBX_X env: ...")` | **boundary** (returned through `config.Load` → cmd) | Migrate to `errcode.Newf("CONFIG.ENV_PARSE_FAILED", ...)` — done in this commit for representative cases; full migration deferred to spec-0.10 lint wave |

### `internal/platform/config/validation.go` (7 sites)

| Pattern | Class | Decision |
|---|---|---|
| `required` / `min` / `max` / `oneof` rule violations | **boundary** | Migrate to `errcode.Newf("CONFIG.VALIDATION_FAILED", ...)` — exemplar migrated; spec-0.10 lint covers rest |

### `internal/platform/config/loader.go` (7 sites)

| Pattern | Class | Decision |
|---|---|---|
| yaml decode / source merge failures | **boundary** | Migrate to `errcode.Wrap("CONFIG.LOAD_FAILED", root, ...)` — exemplar; full pass spec-0.10 |
| inner helper string formatting | **private** | Keep `fmt.Errorf`; `config.Load` wraps at exit |

### `internal/platform/config/admin.go` (7 sites)

| Pattern | Class | Decision |
|---|---|---|
| admin config validate / dump-defaults / sources verb errors | **boundary** (user-visible CLI output) | Migrate to `errcode.Newf("CONFIG.ADMIN_*", ...)` |

### `cmd/opendbx/root.go` (4 sites)

| Pattern | Class | Decision |
|---|---|---|
| flag validation / choice violations | **boundary** | Migrate to `errcode.Newf("CMD.FLAG_INVALID", ...)` |

### `internal/platform/logger/errcode_bridge.go` (1 site)

| Pattern | Class | Decision |
|---|---|---|
| `errcodeFromErr` internal fallback context | **private** (helper inside logger pkg) | Keep `fmt.Errorf`; logger doesn't surface this error externally |

## Summary

- **Total sites**: 40
- **boundary**: 35 (config + cmd)
- **private**: ~3 (loader inner helpers + logger bridge)
- **external**: ~2 (yaml.Decode wrap sites)
- **already-wrapped**: 0 at audit time (no preexisting `errcode.Wrap` in spec-0.5 codebase)

## spec-0.6 T-8 actual state (R2 honesty pass — codex post-impl finding)

**Migration status**: 6 public sentinel migrations (logger 4 + entrypoints 2)
**are** complete and exercised by `errors.go` + `migration_test.go`. The 40
inline `fmt.Errorf` sites are **NOT** migrated in this PR — only their
target boundary codes are registered as the Stage-0 foundation.

### What landed in this PR

1. **6 public sentinel migrations** (D-4): `logger.ErrInvalidLevel` /
   `ErrAlreadyInitialised` / `ErrNotInitialised` / `ErrWriterClosed` +
   `entrypoints.ErrLauncherNotImplemented` /
   `ErrInteractiveHelperNotImplemented` now backed by `errcode.Register`
   with `errors.Is` backward-compat preserved.
2. **4 CONFIG.* boundary codes** registered in `config/errors.go`. These
   are **ready to use** but call sites in `config/{env,validation,loader,
   admin}.go` still use `fmt.Errorf` — leaves the codes as Stage-0 contract
   without churning the diff.
3. **Audit classification** (boundary / private / external / already-wrapped)
   for all 40 sites in this document so the spec-0.10 lint pass has a
   ground-truth reference.

### Deferred to spec-0.10 lint wave

All 40 `fmt.Errorf` inline-new-error sites stay as-is in this PR. The
spec-0.10 custom golangci-lint rule will:

- Scan every package's public boundary surface for `fmt.Errorf` returns
- Flag any boundary error that does not pass through `errcode.Newf` /
  `errcode.Wrap`
- Auto-suggest the right `CONFIG.*` / `CMD.*` code based on this audit
  document's classification

This split honours user Q11 B+ scope decision: **codes registered (this PR)
+ public boundary use enforced (spec-0.10 lint)**. The codes are not dead
weight — they're the canonical Code values the lint rule will recommend.

### New error codes registered in this PR

- `CONFIG.ENV_PARSE_FAILED` — env var value did not match expected shape
- `CONFIG.VALIDATION_FAILED` — schema validation rule violation
- `CONFIG.LOAD_FAILED` — yaml decode / source resolution failed
- `CONFIG.ADMIN_FIELD_NOT_FOUND` — admin config sources field unknown

All registered in `internal/platform/config/errors.go` with full
Message + Hint text. No `CMD.*` codes registered yet — those land with
the corresponding cmd/opendbx migration in a follow-up.
