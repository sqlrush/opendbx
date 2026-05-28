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

## `github.com/anthropics/anthropic-sdk-go` v1.45.0

- **license**: MIT (module LICENSE; codex T-2 verified)
- **maintenance**: active (Anthropic first-party; v1.45.0 released 2026-05-21; ~20 minors in 4 months)
- **alternatives considered**:
  - hand-rolled Anthropic Messages API HTTP/SSE client — rejected in spec-1.20 § 5 (SSE parse / retry / auth / beta-header / API-version churn maintenance cost; hand-rolled SSE was an opendb 痛点 1.1 root cause). **Re-weighed in spec-1.20 R-fix (option C)** once the transitive closure surfaced; user path-3/3 chose to keep the official SDK (option A) — protocol/SSE correctness > supply-chain surface for the 1.20 demoable goal. Hand-rolled narrowing deferred to spec-1.20.1 / 3.x if dep policy tightens.
  - `sashabaranov/go-openai` (OpenAI-compat) — wrong vendor; OpenAI-compat adapter is spec-3.11.
- **risk**: **transitive-closure supply-chain (HIGH, accepted module-graph-only)** — the SDK is a SINGLE module bundling Anthropic + AWS Bedrock + GCP Vertex + MCP backends, so MVS pulls ~60 transitive modules (full `aws-sdk-go-v2`, `cloud.google.com/go/*`, `google.golang.org/{api,grpc,protobuf}`, `modelcontextprotocol/go-sdk`, otel) into `go list -m all` + go.sum. **`go mod why -m` confirms NONE are compiled into the opendbx binary** — only the Claude Messages API path is imported; the rest is module-graph / go.sum surface, not a runtime dependency. spec-1.20 R-fix option-B investigation: no SDK version since v1.25.0 drops AWS/GCP (direct requires of the SDK's own go.mod); bedrock/vertex are same-module subpackages so build-tag/replace cannot prune them; the closure is inherent to the single-module multi-backend design and not avoidable while using this SDK. All transitive modules are version-pinned in `allowlist.json:transitive_lock`. API churn risk medium (fast-moving SDK; pinned v1.45.0, upgrade requires review). x/crypto pulled to v0.40.0 (the spec-0.12-predicted bump; > v0.17.0 so CVE-2022-27191 / CVE-2023-48795 covered).
- **isolation**: IMP-7 (llm-sdk-isolation) — only `internal/domain/llm/anthropic` may import the SDK; app layer is SDK-free via the `llm.Provider` interface (规则 16).
- **go_directive**: SDK requires `go 1.22+`; opendbx is at `go 1.24` so compatible.
- **pkgsite**: https://pkg.go.dev/github.com/anthropics/anthropic-sdk-go@v1.45.0
- **spec_ref**: spec-1.20-llm-client.md § 5 (dep decision) + R-fix option-A (transitive lock)

## `github.com/openai/openai-go` v1.12.0

- **license**: Apache-2.0 (module LICENSE; codex T-2 verified)
- **maintenance**: active (OpenAI first-party, stainless-generated like anthropic-go; v1.x GA line)
- **alternatives considered**:
  - hand-rolled OpenAI `/v1/chat/completions` HTTP/SSE — 痛点 1.1 (hand-rolled SSE) root-cause; rejected spec-1.20.1 Q2 (re-weighed after the Azure closure surfaced in T-2; user path-3/3 kept the official SDK)
  - `sashabaranov/go-openai` — community SDK; diverges from the stainless `ssestream` shape the anthropic adapter already uses
- **risk**: **transitive-closure (accepted module-graph-only)** — openai-go v1.12.0 directly requires Azure SDK (azcore/azidentity/internal) + tidwall; the full graph adds 8 modules (Azure×3 + AzureAD/MSAL + golang-jwt/v5 + google/uuid + kylelemons/godebug + pkg/browser), **DISJOINT from anthropic-go (AWS/GCP)** — no shared stainless runtime as first assumed. **`go mod why -m` confirms Azure/MSAL/pkg-browser are NOT needed for the non-Azure OpenAI-compat path** (deepseek/qwen/glm via api_key+base_url) — module-graph / go.sum surface only, not compiled into that code path. All pinned in `allowlist.json:transitive_lock`.
- **isolation**: IMP-7 (llm-sdk-isolation) — only `internal/domain/llm/openai` may import the SDK (pre-wired in spec-1.20; verified spec-1.20.1); app layer SDK-free via `llm.Provider` (规则 16).
- **go_directive**: requires `go 1.22+`; opendbx at `go 1.24` compatible.
- **pkgsite**: https://pkg.go.dev/github.com/openai/openai-go@v1.12.0
- **spec_ref**: spec-1.20.1-openai-compat.md § 5 + § 1.4 (B-64/65/66 codex VERIFIED) + allowlist transitive_lock
