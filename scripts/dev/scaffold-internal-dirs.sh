#!/usr/bin/env bash
# scaffold-internal-dirs.sh - one-shot scaffolding of opendbx internal/ tree
#
# Creates the full internal/ + tools/ + tests/ + docs/ + pkg/ + scripts/ +
# install/ + configs/ scaffolding per spec-0.2 § 2.1 directory tree.
#
# Idempotent: re-runs do not overwrite existing files (uses `[ -f ... ] || cat`).
#
# Design: opendbrb/specs/stage-0/spec-0.2-go-module-layout.md § 2.1, T-3 task

set -euo pipefail

# Locate opendbx repo root (the script lives at <repo>/scripts/dev/).
REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$REPO_ROOT"

write_doc_go() {
  # Args: <relative-path> <package-name> <one-line-purpose> <spec-ref>
  local relpath="$1"
  local pkg="$2"
  local purpose="$3"
  local spec_ref="$4"

  mkdir -p "$relpath"
  local file="$relpath/doc.go"

  if [ -f "$file" ]; then
    return 0
  fi

  cat > "$file" <<EOF
// Copyright 2026 opendbx contributors. See LICENSE.
//
// Package $pkg $purpose
//
// Design: $spec_ref
// Author: sqlrush
package $pkg
EOF
}

write_readme() {
  # Args: <relative-path> <body-file-or-heredoc>
  local relpath="$1"
  local body="$2"
  mkdir -p "$relpath"
  local file="$relpath/README.md"
  if [ -f "$file" ]; then
    return 0
  fi
  printf '%s\n' "$body" > "$file"
}

# ---- internal/bootstrap, entrypoints ----
write_doc_go "internal/bootstrap"   "bootstrap"   "wires application lifecycle and dependency injection."  "spec-0.3 ~ spec-0.6"
write_doc_go "internal/entrypoints" "entrypoints" "selects and dispatches CLI subcommand entrypoints."     "spec-0.3-cmd-entrypoints"

# ---- internal/app/cli ----
write_doc_go "internal/app/cli/tui"                "tui"        "TUI shell wiring tcell event loop to render engine." "spec-1.15-tui"
write_doc_go "internal/app/cli/render/terminal"    "terminal"   "tcell terminal abstraction (low-level driver)."     "spec-0.12-tcell-bootstrap"
write_doc_go "internal/app/cli/render/buffer"      "buffer"     "double-buffered cell grid for diff-based redraw."   "spec-1.2-render-buffer"
write_doc_go "internal/app/cli/render/layout"      "layout"     "Yoga-like flex layout primitives."                  "spec-1.1-layout-engine"
write_doc_go "internal/app/cli/render/optimizer"   "optimizer"  "render diff optimizer (skip unchanged cells)."      "spec-1.3-render-optimizer"
write_doc_go "internal/app/cli/render/scheduler"   "scheduler"  "frame scheduler with goroutine pool."               "spec-1.4-render-scheduler"
write_doc_go "internal/app/cli/render/scrollback"  "scrollback" "27k-line scrollback buffer with virtual scroll."   "spec-1.5-scrollback"
write_doc_go "internal/app/cli/render/streaming"   "streaming"  "streaming token append (no reflow of completed lines)." "spec-1.6-streaming-incremental"
write_doc_go "internal/app/cli/render/block"       "block"      "block renderers (message / toolcall / markdown / code / diff / ...)." "spec-1.7 ~ spec-1.13"
write_doc_go "internal/app/cli/render/style"       "style"      "self-built lipgloss-like style primitives."         "spec-1.13-style"
write_doc_go "internal/app/cli/render/width"       "width"      "string width with EastAsianWidth=false invariant."  "spec-1.14-string-width"
write_doc_go "internal/app/cli/input"              "input"      "three-mode input (insert/normal/visual)."           "spec-1.16-input-three-modes"
write_doc_go "internal/app/cli/keybindings"        "keybindings" "keymap dispatch and customization."                "spec-1.17-keybindings"
write_doc_go "internal/app/cli/components"         "components" "high-level CLI components (picker / spinner / table)." "Stage 1+"

# ---- internal/app top-level subsystems ----
write_doc_go "internal/app/commands/builtin"       "builtin"    "built-in slash commands (CC ecosystem alignment)."  "spec-2.18-builtin-commands"
write_doc_go "internal/app/commands/bundled"       "bundled"    "opendbx-bundled DB diagnostic commands."            "spec-2.18-bundled-commands"
write_doc_go "internal/app/commands/plugin"        "plugin"     "external SKILL.md-injected slash commands."         "spec-2.1-skill-md-format"
write_doc_go "internal/app/skills"                 "skills"     "SKILL.md loader and Skill interface."               "spec-2.1 ~ spec-2.4"
write_doc_go "internal/app/hooks"                  "hooks"      "user hook events (PreToolUse / PostToolUse / SessionStart / etc.)." "spec-2.8 ~ spec-2.9"
write_doc_go "internal/app/tools"                  "tools"      "tool registry (one directory per tool, Stage 1+)."  "spec-2.X-tools"
write_doc_go "internal/app/outputstyles"           "outputstyles" "output style themes."                             "spec-2.17-output-styles"
write_doc_go "internal/app/plan"                   "plan"       "plan mode (CC-aligned)."                            "spec-2.14-plan-mode"
write_doc_go "internal/app/todo"                   "todo"       "todo tracking (CC-aligned)."                        "spec-2.15-todo"
write_doc_go "internal/app/subagent"               "subagent"   "subagent spawning (CC-aligned)."                    "spec-2.16-subagent"
write_doc_go "internal/app/statusline"             "statusline" "CC status line (north-star CC ecosystem alignment)." "spec-2.X-statusline (AD-005)"
write_doc_go "internal/app/permissions"            "permissions" "CC-aligned permissions UI."                        "spec-2.X-permissions (AD-005)"
write_doc_go "internal/app/plugins"                "plugins"    "CC-aligned plugin manager."                         "spec-2.X-plugins (AD-005)"
write_doc_go "internal/app/autopilot/cerebrate"    "cerebrate"  "Stage 9+ autopilot top-tier agent (cluster brain)." "Stage 9+"
write_doc_go "internal/app/autopilot/overlord"     "overlord"   "Stage 9+ mid-tier coordinator agent."               "Stage 9+"
write_doc_go "internal/app/autopilot/drone"        "drone"      "Stage 9+ leaf-tier worker agent."                   "Stage 9+"
write_doc_go "internal/app/sentinel"               "sentinel"   "DB metric sentinel probes (48 metrics in MVP)."     "Stage 1 skeleton + Stage 3 full"
write_doc_go "internal/app/diagnose"               "diagnose"   "LLM-driven multi-round DB diagnosis loop."          "spec-1.21-diagnose-loop"
write_doc_go "internal/app/report"                 "report"     "AWR-style report generation."                       "spec-1.23 + spec-3.7"
write_doc_go "internal/app/services/mcp"           "mcp"        "MCP server/client implementation."                  "spec-2.5 ~ spec-2.7"
write_doc_go "internal/app/services/auth"          "auth"       "authentication service (Stage 2+)."                 "Stage 2+"
write_doc_go "internal/app/services/costtracker"   "costtracker" "LLM cost tracker with budget enforcement."         "spec-3.8-cost-tracker"
write_doc_go "internal/app/services/notifier"      "notifier"   "notification dispatch (absorbs opendb alert)."      "Stage 1+"
write_doc_go "internal/app/services/settingssync"  "settingssync" "settings sync service."                           "spec-2.X-settings-sync"
write_doc_go "internal/app/services/teammemsync"   "teammemsync" "team memory sync service."                         "spec-2.11-team-memory"
write_doc_go "internal/app/services/dbpool"        "dbpool"     "DB connection pool service."                        "Stage 1"
write_doc_go "internal/app/services/upstreamproxy" "upstreamproxy" "enterprise LLM upstream proxy."                  "spec-3.9-upstream-proxy"
write_doc_go "internal/app/services/remote"        "remote"     "remote session service."                            "spec-2.6 + Stage 9+"
write_doc_go "internal/app/state"                  "state"      "global UI store (TUI render-loop state)."           "spec-1.15-tui (state submodule)"

# ---- internal/domain ----
write_doc_go "internal/domain/db"                  "db"         "database driver interface (provider-agnostic)."     "spec-1.18-pg-driver"
write_doc_go "internal/domain/db/postgres"         "postgres"   "PostgreSQL driver (pgx/v5, MVP only real driver)."  "spec-1.18-pg-driver"
write_doc_go "internal/domain/db/mysql"            "mysql"      "MySQL driver (Stage 6 reserved)."                   "Stage 6 (reserved)"
write_doc_go "internal/domain/db/oracle"           "oracle"     "Oracle driver (Stage 7 reserved)."                  "Stage 7 (reserved)"
write_doc_go "internal/domain/db/opengauss"        "opengauss"  "openGauss driver (Stage 8 reserved)."               "Stage 8 (reserved)"
write_doc_go "internal/domain/llm"                 "llm"        "LLM provider interface (Anthropic / OpenAI / etc.)." "spec-1.20-llm-client"
write_doc_go "internal/domain/llm/anthropic"       "anthropic"  "Anthropic SDK adapter (provider implementation)."   "spec-1.20-llm-client"
write_doc_go "internal/domain/llm/openai"          "openai"     "OpenAI-compat adapter (covers GLM / Qwen / DeepSeek)." "spec-1.20-llm-client"
write_doc_go "internal/domain/llm/model"           "model"      "model tier configuration (Tier 1 ~ 4)."             "spec-3.11-model-tiers"
write_doc_go "internal/domain/memory"              "memory"     "memory storage and retrieval (with fingerprinting)." "spec-2.10 ~ spec-2.11"
write_doc_go "internal/domain/session"             "session"    "session storage (current.jsonl + audit/ split)."    "spec-2.12 ~ spec-2.13"
write_doc_go "internal/domain/trace"               "trace"      "trace span and tag domain."                         "Stage 0 + Stage 1"
write_doc_go "internal/domain/security"            "security"   "security domain (credentials / audit / redaction)." "Stage 4"
write_doc_go "internal/domain/security/credential" "credential" "credential vault (encrypted at rest)."              "spec-4.3-credential-vault"
write_doc_go "internal/domain/cluster"             "cluster"    "cluster protocol domain (Stage 9+)."                "Stage 9+"
write_doc_go "internal/domain/schemas"             "schemas"    "centralized JSON schema literals (pure data)."      "spec-0.2 § 2.1"

# ---- internal/platform ----
write_doc_go "internal/platform/config"            "config"     "tiered config (user / dev / system)."               "spec-0.4-config-framework"
write_doc_go "internal/platform/logger"            "logger"     "structured logger with PII redaction."              "spec-0.5-logging"
write_doc_go "internal/platform/apperr"            "apperr"     "Code/Message/Hint error triple (replaces stdlib errors patterns)." "spec-0.6-error-codes"
write_doc_go "internal/platform/version"           "version"    "build version string (cmd/opendbx --version reads here)." "spec-0.7-version-numbering"
write_doc_go "internal/platform/osutil"            "osutil"     "cross-platform OS helpers (paths, signals, env)."   "Stage 0+"
write_doc_go "internal/platform/rpc"               "rpc"        "gRPC / HTTP / Unix Socket transport framework."     "Stage 2+"
write_doc_go "internal/platform/migrations"        "migrations" "version-upgrade SQLite/state migrations (bootstrap-only import)." "spec-4.8-version-migrations"

# ---- pkg ----
mkdir -p pkg
[ -f pkg/README.md ] || cat > pkg/README.md <<'EOF'
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
EOF

# ---- tests/ ----
write_readme "tests/integration" "# tests/integration

Integration tests with testcontainers (real PG + fake LLM provider).

Layered per CLAUDE.md § 3.9 + spec-0.2 § 2.1:
- \`uitest/\` — PTY + cell golden (CLAUDE.md § 3.9 Layer 2)

Run: \`go test ./tests/integration/...\`
"

write_readme "tests/integration/uitest" "# tests/integration/uitest

PTY-based UI tests using \`creack/pty\` + \`hinshun/vt10x\` per CLAUDE.md § 3.9
Layer 2. Cell-grid metadata compared against golden files.

Reserved for spec-0.11.5-ui-review-pipeline.
"

write_readme "tests/e2e" "# tests/e2e

End-to-end tests with real LLM + real PG.

Layered per CLAUDE.md § 3.9:
- \`uitest-visual/\` — \`charmbracelet/freeze\` ANSI→PNG + pixelmatch (CLAUDE.md § 3.9 Layer 3)

Run: \`go test -tags=e2e ./tests/e2e/...\`
"

write_readme "tests/e2e/uitest-visual" "# tests/e2e/uitest-visual

Visual pixel-level tests. ANSI captured frames rendered to PNG via
\`charmbracelet/freeze\`, diffed via \`pixelmatch\` (threshold < 1%).

Reserved for spec-0.11.5-ui-review-pipeline.
"

write_readme "tests/chaos" "# tests/chaos

Chaos engineering / fault injection (nightly only).

Categories per CLAUDE.md § 4 Layer 4:
- network partition / disk full / OOM / LLM empty response

Reserved for Stage 1+ (spec-1.X-chaos-* introduces concrete cases).
"

write_readme "tests/perf" "# tests/perf

Performance benchmarks. Each Stage end freezes a baseline JSON
(\`docs/perf/baseline-stage<N>.json\`) and CI compares against it
(per CLAUDE.md rule 11).

> 3% regression → WARN. > 5% → FAIL + RCA.
"

# fixtures/{9 domains}
for domain in llm session sentinel ui db engine perf security cluster; do
  case "$domain" in
    llm)      desc="LLM benchmark prompts + multi-model response samples (catalog L-*)" ;;
    session)  desc="session SQL samples and audit fixtures (catalog S-*)" ;;
    sentinel) desc="sentinel probe baseline data (48 metrics, catalog ST-*)" ;;
    ui)       desc="UI fixtures (PTY golden / visual PNG, catalog U-*)" ;;
    db)       desc="DB schema templates (MVP only postgres/, catalog DB-*)" ;;
    engine)   desc="LLM engine pipeline fixtures (catalog E-*)" ;;
    perf)     desc="performance baseline JSON files (catalog P-*)" ;;
    security) desc="redaction / audit fixtures (catalog C-*)" ;;
    cluster)  desc="cluster mode fixtures (Stage 9+)" ;;
  esac
  write_readme "tests/fixtures/$domain" "# tests/fixtures/$domain

$desc.

Filename convention: \`<catalog-id>-<slug>.<ext>\` (e.g.
\`L-3-truncation-recovery.json\`).
"
done

# ---- scripts/ ----
write_readme "scripts/perf" "# scripts/perf

Performance baseline generation and comparison utilities.

- \`baseline.sh\` (Stage 1+) — freeze stage baseline JSON
- \`compare.sh\` (Stage 1+) — compare run vs baseline + benchstat output
"

write_readme "scripts/release" "# scripts/release

Release automation (spec-5.1).

- \`tag.sh\` — bump version + create signed tag
- \`changelog.sh\` — generate release notes from CHANGELOG [Unreleased]
"

write_readme "scripts/dev" "# scripts/dev

Developer-only helpers (not run in CI).

- \`scaffold-internal-dirs.sh\` — one-shot scaffolding of internal/ tree (spec-0.2 T-3)
"

# ---- configs/, install/ ----
for env in dev test prod; do
  write_readme "configs/$env" "# configs/$env

\`$env\` environment configuration (YAML schema by spec-0.4-config-framework).

Reserved — actual schema lives in spec-0.4 implementation.
"
done

for os in darwin linux; do
  write_readme "install/$os" "# install/$os

\`$os\` installer assets (homebrew formula / .deb / .rpm / etc.).

Reserved — actual install scripts land in spec-4.7-install.
"
done

# ---- docs/dependencies/ ----
# Note: this script does NOT regenerate docs/dependencies/{README.md,allowlist.json}.
# Those files are committed to the repo as authoritative; previous versions of
# this scaffold held an embedded fallback that drifted from the real allowlist
# (codex M-08). If the files are missing, fail loudly instead of writing stale
# templates.
if [ ! -f docs/dependencies/README.md ] || [ ! -f docs/dependencies/allowlist.json ]; then
  echo "ERR: docs/dependencies/{README.md,allowlist.json} missing." >&2
  echo "     They are committed to the repo; restore from git rather than running scaffold." >&2
  exit 1
fi

echo "scaffold complete."
echo ""
echo "Counts:"
echo "  doc.go files:   $(find internal -name doc.go | wc -l | tr -d ' ')"
echo "  tests README:   $(find tests -name README.md | wc -l | tr -d ' ')"
echo "  scripts README: $(find scripts -name README.md -mindepth 2 | wc -l | tr -d ' ')"
echo "  configs README: $(find configs -name README.md -mindepth 2 | wc -l | tr -d ' ')"
echo "  install README: $(find install -name README.md -mindepth 2 | wc -l | tr -d ' ')"
