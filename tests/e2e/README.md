# tests/e2e

End-to-end tests with real LLM + real PG.

Layered per CLAUDE.md § 3.9:
- `uitest-visual/` — `charmbracelet/freeze` ANSI→PNG + pixelmatch (CLAUDE.md § 3.9 Layer 3)

Run: `go test -tags=e2e ./tests/e2e/...`

