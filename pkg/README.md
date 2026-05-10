# `pkg/` — public API surface (intentionally empty in spec-0.2)

`pkg/` is reserved for future public API surface (Go packages importable by
third-party code).

## Why empty as of spec-0.2

opendbx (Stage 0 ~ Stage 5) is a single self-contained binary. No external
consumer needs to import opendbx packages. All code lives under `internal/`,
which the Go compiler enforces as private (any external import attempt fails).

Premature exposure → breaking changes later. We keep `pkg/` empty until a
**concrete external consumer + maintained API contract** is justified.

## When `pkg/` will be populated

Any addition requires a spec decision (CLAUDE.md rule 6 + spec § 8 Q&A). Likely
candidates:

- `pkg/skillsdk/` — interfaces for SKILL.md authors (post spec-2.1 stabilization)
- `pkg/mcpsdk/` — MCP server/client SDK for third-party integrations
- `pkg/sentinelsdk/` — sentinel probe API for custom rule authors

## Anti-pattern

Do **not** treat `pkg/` as a "common types" dumping ground. Anything that is
"common to several internal packages" belongs in `internal/domain/` or
`internal/platform/`, not `pkg/`.

## Reference

- spec-0.2-go-module-layout.md § 2.1 / § 8 Q5 (decision: keep empty)
- CLAUDE.md rule 6 (dependency / surface management)
