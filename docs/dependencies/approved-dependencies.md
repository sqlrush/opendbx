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
