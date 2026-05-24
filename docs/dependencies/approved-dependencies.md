# Approved dependencies — evaluation cards

> spec-0.12 D-1 introduces this file as the SSOT for **per-dependency
> evaluation cards** (license / maintenance / alternatives / risk).
> Distinct from `allowlist.json` (3-field schema for the
> dep-allowlist-check tool); this file is documentation, not a lint
> input.

Each card answers four questions: **license** (permissive?), **maintenance**
(active upstream?), **alternatives considered** (why this one?), **risk**
(supply-chain / API churn / lock-in concerns?).

Adding a new card requires the same spec gating as adding to
`allowlist.json:direct_allowed` — reference the introducing spec in the
card's `spec_ref` field.

---

## `github.com/gdamore/tcell/v2` v2.13.9

- **license**: Apache-2.0
- **maintenance**: active (last release 2026-04-20 per pkg.go.dev; ~10 year mature project)
- **alternatives considered**:
  - `rivo/tview` — too high-level, conflicts with AD-002 self-built engine
  - `charmbracelet/bubbletea` — explicitly excluded by AD-002 + CLAUDE.md § 3.1
  - fork bubbletea — modification scope ≈ self-build with no upstream upside
- **risk**: tcell v2 supply-chain risk low; Windows ConsoleAPI legacy path
  not applicable (opendbx Stage 0 ships macOS + Linux only). API surface is
  stable across 2.x; major version bumps follow semver.
- **go_directive**: `go 1.24.0` (drives opendbx toolchain bump from 1.23 →
  1.24 per spec-0.12 R2 CRIT-1)
- **pkgsite**: https://pkg.go.dev/github.com/gdamore/tcell/v2@v2.13.9
- **spec_ref**: spec-0.12-tcell-bootstrap.md § 2

## `github.com/yuin/goldmark` v1.7.8

- **license**: MIT
- **maintenance**: active (last release v1.7.8 2024-12-15 per `go list -m -json`; CommonMark spec-compliant; widely adopted in Go ecosystem)
- **alternatives considered**:
  - `russross/blackfriday/v2` — archived 2024, no new features, GFM partial → rejected
  - `gomarkdown/markdown` — fork of blackfriday; smaller community, AST less stable → rejected
  - `markdown-it-go` — JS port; unfamiliar API, less idiomatic Go → rejected
- **risk**: API churn risk low (1.x line mature 4+ years); AST node-type changes on major bump would require walker updates — see spec-1.11 R-1. Transitive closure: 0 new transitive deps (goldmark stdlib-only); extensions in same `github.com/yuin/goldmark/extension` module (Table + Linkify enabled per Q2 ★C).
- **go_directive**: requires `go 1.19+`; opendbx is at `go 1.24` so compatible.
- **pkgsite**: https://pkg.go.dev/github.com/yuin/goldmark@v1.7.8
- **spec_ref**: spec-1.11-markdown-block.md § 5 (D-5 dep contract)

## `github.com/alecthomas/chroma/v2` v2.24.1

- **license**: MIT (engine) + OFL-1.1 (font/style entries)
- **maintenance**: active (last release v2.24.1 2026-04-30 per pkg.go.dev; widely adopted Go ecosystem; pygments-port complete; 250+ lexers)
- **alternatives considered**:
  - `alecthomas/syntax-highlight` — archived 2023 → rejected
  - pygments-go fork — calls out to python interpreter; binary dist concern → rejected
  - highlight.js wasm bridge — wasm runtime overhead; ABI churn → rejected (spec-1.12 ❌-3)
- **risk**: API churn risk low (2.x line mature 4+ years); minor bumps may change StyleEntry palette nuances — see spec-1.12 R-1/R-3. Transitive: 1 new direct (`github.com/dlclark/regexp2`); 3 test-only transients (`alecthomas/assert/v2` / `alecthomas/repr` / `hexops/gotextdiff`).
- **go_directive**: requires `go 1.20+`; opendbx is at `go 1.24` so compatible.
- **pkgsite**: https://pkg.go.dev/github.com/alecthomas/chroma/v2@v2.24.1
- **spec_ref**: spec-1.12-code-highlight-block.md § 5 (D-1 dep contract)
