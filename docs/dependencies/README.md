# `docs/dependencies/` — dependency allowlist

opendbx enforces a 3-tier dependency allowlist per spec-0.2 § 2.5 + CLAUDE.md
rule 6. The allowlist is JSON (not YAML) so that `tools/dep-allowlist-check`
parses it with stdlib `encoding/json` and the only third-party tool dep is
`golang.org/x/tools` (the unique tool-only example approved in spec-0.2).

## `allowlist.json` shape

```json
{
  "direct_allowed": [
    { "module": "github.com/example/foo", "purpose": "...", "introduced_by": "spec-X.Y-slug" }
  ],
  "transitive_lock": [
    { "module": "golang.org/x/text", "version": "v0.14.0" }
  ],
  "tool_only": [
    { "module": "golang.org/x/tools", "purpose": "...", "introduced_by": "spec-0.2" }
  ]
}
```

Top-level keys starting with `_` are reserved for inline comments (JSON has
no native comment syntax). Any other unknown top-level key fails the load.

## Tiers

1. **`direct_allowed`** — modules that opendbx explicitly `require`s.
   Adding requires a spec decision (`introduced_by` must reference a real
   spec id). Enforced by `tools/dep-allowlist-check`.

2. **`transitive_lock`** — snapshot of indirect modules pulled by
   `direct_allowed` deps. Updates require human review (compare diff vs
   previous lock snapshot before merging).

3. **`tool_only`** — modules importable **only** by code under `tools/`.
   Production code (`cmd/`, `internal/`) must not import these.

A module legitimately may appear in both `direct_allowed` (forward contract
for a future spec) AND `transitive_lock` (current actual via another dep).
Cross-tier dual listing is allowed; intra-tier duplicates fail.

## Workflow when adding a new dep

1. Spec mentions the dep in its § 5 contract with `introduced_by: spec-X.Y`.
2. PR adds the dep to the relevant tier in `allowlist.json`.
3. `go get` and `go mod tidy` are run.
4. CI `dep-check` job validates `go list -m -json all` against
   `allowlist.json`.
5. If transitive deps changed, update `transitive_lock` in the same PR.

## Reference

- `tools/dep-allowlist-check/` — enforcement binary
- spec-0.2-go-module-layout.md § 2.4 / § 2.5
- CLAUDE.md rule 6 (dependency management)
