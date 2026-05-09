# tests/integration

Integration tests with testcontainers (real PG + fake LLM provider).

Layered per CLAUDE.md § 3.9 + spec-0.2 § 2.1:
- `uitest/` — PTY + cell golden (CLAUDE.md § 3.9 Layer 2)

Run: `go test ./tests/integration/...`

