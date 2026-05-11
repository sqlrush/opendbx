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
| ENV parse errors (uint8/int/duration/bool/oneof) | `fmt.Errorf("OPENDBX_X env: ...")` | **boundary** (returned through `config.Load` → cmd) | `config.Load` wraps with `CONFIG.ENV_PARSE_FAILED`; helper internals remain `fmt.Errorf` |

### `internal/platform/config/validation.go` (7 sites)

| Pattern | Class | Decision |
|---|---|---|
| `required` / `min` / `max` / `oneof` rule violations | **boundary** | `config.Load` / `ValidateFile` wrap with `CONFIG.VALIDATION_FAILED`; helper internals remain `fmt.Errorf` |

### `internal/platform/config/loader.go` (7 sites)

| Pattern | Class | Decision |
|---|---|---|
| yaml decode / source merge failures | **boundary** | `config.Load` wraps with `CONFIG.LOAD_FAILED` |
| inner helper string formatting | **private** | Keep `fmt.Errorf`; `config.Load` wraps at exit |

### `internal/platform/config/admin.go` (7 sites)

| Pattern | Class | Decision |
|---|---|---|
| admin config validate / dump-defaults / sources verb errors | **boundary** (user-visible CLI output) | Wrapped with `CONFIG.*` / `ERRCODE.*` at admin API boundary |

### `cmd/opendbx/root.go` (4 sites)

| Pattern | Class | Decision |
|---|---|---|
| flag validation / choice violations | **boundary** | Wrapped with `CMD.FLAG_INVALID` |

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
**are** complete and exercised by `errors.go` + migration tests. Public
boundary returns for config/admin/cmd now wrap their existing helper errors
with `CONFIG.*`, `CMD.FLAG_INVALID`, or builtin `ERRCODE.*` codes. Private
helpers still use `fmt.Errorf`; that is intentional under the B+ scope because
the exported boundary adds the code.

### What landed in this PR

1. **6 public sentinel migrations** (D-4): `logger.ErrInvalidLevel` /
   `ErrAlreadyInitialised` / `ErrNotInitialised` / `ErrWriterClosed` +
   `entrypoints.ErrLauncherNotImplemented` /
   `ErrInteractiveHelperNotImplemented` now backed by `errcode.Register`
   with `errors.Is` backward-compat preserved.
2. **4 CONFIG.* boundary codes** registered in `config/errors.go` and used at
   `config.Load` / admin config public boundaries.
3. **Audit classification** (boundary / private / external / already-wrapped)
   for all 40 sites in this document so the spec-0.10 lint pass has a
   ground-truth reference.
4. **CMD.FLAG_INVALID** registered via `errcode.ErrFlagInvalid` and used by
   cmd/opendbx choice flag validation.

### Deferred to spec-0.10 lint wave

Private `fmt.Errorf` inline-new-error sites stay as-is. The spec-0.10 custom
golangci-lint rule will:

- Scan every package's public boundary surface for `fmt.Errorf` returns
- Flag any boundary error that does not pass through `errcode.Newf` /
  `errcode.Wrap`
- Auto-suggest the right `CONFIG.*` / `CMD.*` / `ERRCODE.*` code based on this audit
  document's classification

This split honours user Q11 B+ scope decision: **public boundaries carry
codes now; private helper style is enforced later by lint**.

### New error codes registered in this PR

- `CONFIG.ENV_PARSE_FAILED` — env var value did not match expected shape
- `CONFIG.VALIDATION_FAILED` — schema validation rule violation
- `CONFIG.LOAD_FAILED` — yaml decode / source resolution failed
- `CONFIG.ADMIN_FIELD_NOT_FOUND` — admin config sources field unknown
- `CMD.FLAG_INVALID` — command-line flag value failed choice/range validation

All registered in `internal/platform/config/errors.go` with full
Message + Hint text, except `CMD.FLAG_INVALID` which lives in
`internal/platform/errcode/builtin.go` so docs generation can load it without
importing a command package.
