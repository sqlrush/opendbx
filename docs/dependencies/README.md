# `docs/dependencies/` — dependency allowlist

opendbx enforces a 3-tier dependency allowlist per spec-0.2 § 2.5 + CLAUDE.md
rule 6.

## `allowlist.yml` shape

```yaml
direct_allowed:
  - module: github.com/example/foo
    introduced_by: spec-X.Y-slug
transitive_lock:
  - module: golang.org/x/text
    version: v0.14.0
tool_only:
  - module: golang.org/x/tools
    introduced_by: spec-0.2
```

## Tiers

1. **`direct_allowed`** — modules that opendbx explicitly `require`s.
   Adding requires a spec decision (`introduced_by` must reference a real
   spec id). Enforced by `tools/dep-allowlist-check`.

2. **`transitive_lock`** — snapshot of indirect modules pulled by
   `direct_allowed` deps. Updates require human review (compare diff vs
   previous lock snapshot before merging).

3. **`tool_only`** — modules importable **only** by code under `tools/`.
   Production code (`cmd/`, `internal/`) must not import these.

## Workflow when adding a new dep

1. Spec mentions the dep in its § 5 contract with `introduced_by: spec-X.Y`.
2. PR adds the dep to the relevant tier in `allowlist.yml`.
3. `go get` and `go mod tidy` are run.
4. CI `dep-check` job validates `go list -m -json all` against `allowlist.yml`.
5. If transitive deps changed, update `transitive_lock` in the same PR.

## Reference

- `tools/dep-allowlist-check/` — enforcement binary
- spec-0.2-go-module-layout.md § 2.4 / § 2.5
- CLAUDE.md rule 6 (dependency management)
