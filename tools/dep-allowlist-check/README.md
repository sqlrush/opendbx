# tools/dep-allowlist-check

Self-contained Go binary that validates opendbx's dependency graph against
`docs/dependencies/allowlist.json` per **spec-0.2 § 2.4 / § 2.5** + spec-0.4
D-13 (Tests:true fix).

## Usage

```bash
# from opendbx repo root:
go run ./tools/dep-allowlist-check -v .
# exits 0 on clean, 1 on violation, 2 on fatal scan error
```

CI invokes this in the **Dep Allowlist Check** job.

## Three-tier allowlist

`docs/dependencies/allowlist.json`:

- **`direct_allowed`** — modules that opendbx explicitly `require`s.
  Adding requires a spec decision (`introduced_by` references the spec id).
- **`transitive_lock`** — snapshot of indirect modules pulled by direct
  deps. Updates require human review (compare diff vs previous lock).
- **`tool_only`** — modules importable **only** by code under `tools/`.
  Production code (`cmd/`, `internal/`, `tests/`, `pkg/`) must not import
  these.

## What this tool checks

1. Every direct require in `go.mod` is listed under `direct_allowed`
   (with `introduced_by`).
2. Every indirect (transitive) module is listed under `transitive_lock`
   (with version).
3. `tool_only` modules don't appear in non-`tools/` packages.

## spec-0.4 D-13 R3 fix

`packages.Config{Tests: true}` (was `false` in spec-0.3). _test.go files
also participate in tool_only enforcement now. Without this, a malicious or
mistaken developer could `import _ "golang.org/x/tools/..."` from
`internal/app/foo/foo_test.go` and the tool wouldn't catch it.

## Cross-tier dual listing

A module legitimately may appear in both `direct_allowed` (forward contract
for a future spec) AND `transitive_lock` (current actual via another dep).
This is allowed; the loader validates only intra-tier duplicates.

## Workflow when adding a new dep

1. Spec mentions the dep in its § 5 contract with `introduced_by: spec-X.Y`.
2. PR adds the dep to the relevant tier in `allowlist.json`.
3. Run `go get` and `go mod tidy`.
4. CI `dep-check` job validates `go list -m -json all` against the
   allowlist.
5. If transitive deps changed, update `transitive_lock` in the same PR.

## Related specs

- spec-0.2 § 2.4 / § 2.5 (allowlist tiers + tool design)
- spec-0.3 D-1 (cobra direct + 8 transitive added)
- spec-0.4 D-1 (yaml.v3 promoted to direct)
- spec-0.4 D-13 (this Tests:true fix)
