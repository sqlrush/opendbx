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

## spec-0.6 T-8 actually-migrated subset

To demonstrate the pattern + close the most visible CLI errors, this commit
migrates **5 exemplar sites** spanning the 3 main packages:

1. `config/env.go` — `OPENDBX_OUTPUT_FORMAT` parse error (one of 14 ENV sites)
2. `config/validation.go` — required-field check (one of 7 validation sites)
3. `config/loader.go` — yaml decode wrap at `Load()` exit (one of 7 loader sites)
4. `config/admin.go` — `admin config sources NoSuch.Field` error (one of 7 admin sites)
5. `cmd/opendbx/root.go` — choice validation failure (one of 4 cmd sites)

The remaining 35 `fmt.Errorf` sites are **knowingly deferred** to the
spec-0.10 lint enforcement wave: when the custom golangci-lint rule lands,
it will flag all remaining public-boundary callers and force a sweep. This
honours user Q11 B+ scope decision ("public boundary 强制 / private 自由 /
45 处分类 / spec-0.10 lint enforcement").

## New error codes added in this commit

- `CONFIG.ENV_PARSE_FAILED` — env var value did not match expected shape
- `CONFIG.VALIDATION_FAILED` — schema validation rule violation
- `CONFIG.LOAD_FAILED` — yaml decode / source resolution failed
- `CONFIG.ADMIN_FIELD_NOT_FOUND` — admin config sources field unknown
- `CMD.FLAG_INVALID` — cobra flag value rejected by choice validator

All registered in `internal/platform/config/errors.go` and (for CMD)
`cmd/opendbx/errors.go`.
