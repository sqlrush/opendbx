# Changelog — opendbx

按时间倒序记录代码仓的变更。每个 spec 完成 / stage 验收 / release 都追加一行。

格式参考 [Keep a Changelog](https://keepachangelog.com/)。

---

## [Unreleased]

### FROZEN 2026-05-14: spec-0.10-lint-static-analysis — opendbx be985bd + opendbrb 1580e71

- [spec-0.10-lint-static-analysis] FROZEN — tag `v0.10.0-stage0.10` (spec-0.7 D-2 dual-repo 自动化)
- 6 deliverable: D-1 `.golangci.yml` 9 精选 linters (errcheck/govet/staticcheck/gosec/revive/errorlint/ineffassign/nilnil/unused + nolintlint meta) + settings hardening + D-2 `tools/errcode-lint` Standalone Go binary (spec-0.6 D-4 forward, public API errcode permanent enforcement; AST-based reachingAssignSource local-var detection + classifyReturnExpr 3-way classification + audit manifest 40 sites + invariant test) + D-2.5 `tools/suppression-lint` AST-based (parser.ParseFile + ParseComments) 替代 nolintlint 局限 (4 family spec_ref protocol enforcement: nolint / nosec / errcode-lint:exempt / govulncheck-exempt) + D-3 import-rules-check 4 new rules (IMP-5 opendb-ban / IMP-6 render-cascade / IMP-7 llm-sdk-isolation with LLMSDKRoots boundary-safe / IMP-8 runewidth-wrap) + D-4 opendbrb-commit-lint 4 new check (Spec footer regex / scope regex / BREAKING CHANGE iff strict + BANG_NO_BREAKING_BODY / line-length) + D-5 pre-commit hook 调 bin/ binary + `make lint-all` + CI validate 接入 + D-6 opendbrb `docs/lint-policy.md` SSOT (~190 LOC, 6 节)
- Tool tier 覆盖率 ≥90%: errcode-lint / suppression-lint / opendbrb-commit-lint 均满足 spec-0.8 D-1 四档 tier 门槛
- 全程评审 4 round 共 46 unique finding 全消化:
  - R1 b38baf7 — initial 13 节 draft (522 行)
  - R2 1bdb5f7 — 双路 pre-impl review 33 finding (codex 11H/10M/8L/4N + claude 3H/3M/2L/2N dedup ≈ 33)
  - T-13 1580e71 + be985bd — 三路 post-impl review 13 finding (go-reviewer CRIT-1 suppression-lint isRoot walker 静默 0 文件 / codex HIGH-1 errcode-lint local-var bare-error detection / 三路 HIGH-2 commit-lint 5 新 code exit 1 / codex MED-1 audit manifest / codex MED-2 BREAKING iff strict / go-reviewer MED-2 suppression-lint AST refactor 消除 6 false positive / codex LOW-1 LLMSDKRoots boundary-safe + 5 retrofit nolint spec_ref + 2 new bad fixtures)
- 三路 trace: opendbrb `docs/reviews/{codex,claude-code-reviewer,go-reviewer}-spec-0.10-{pre,post}-impl-review-2026-05-14.md` (6 trace 全集)
- CLAUDE.md § 9 双 trace 最小: T-13 提供 3 trace 满足要求

### 2026-05-14: spec-0.9 post-FROZEN review follow-up — CI hardening

- `make vuln-check` now installs and verifies pinned `govulncheck@v1.1.4` before scanning, so CI cannot accidentally reuse a preinstalled scanner with a different version.
- `sync-branch-protection.sh --dry-run` now fails loudly on GitHub API/network/permission errors and only treats explicit 404/not-protected responses as "no protection configured".

### FROZEN 2026-05-14: spec-0.9-ci-github-actions — opendbx + opendbrb (post-T13)

- [spec-0.9-ci-github-actions] FROZEN — tag `v0.9.0-stage0.9` (spec-0.7 D-2 dual-repo 自动化)
- 8 deliverable: D-1 ci.yml 9 stable-name required job + D-2 security gosec @v2.21.4 + SARIF + D-2.5 vuln-allowlist Go 工具 + D-3 perf-smoke WARN-only + D-4 nightly-chaos 4 stub + D-5 sync-branch-protection + ci-protection-check + D-6 opendbrb docs/cicd-required-jobs.md + D-7 retention 规约
- 9 required stable contexts: validate / build-linux / build-macos / unit-test / import-rules / dependency-policy / cli-golden / security / perf-smoke
- 双 Go 工具覆盖率：vuln-allowlist 92.6% / ci-protection-check 92.6% (Tool tier ≥90%)
- R2 pre-impl 27 + T-7.5 Round 1 mid-impl 11 + T-13 Round 2 post-impl 6 = **44 finding 全消化**（三路 reviewer × 3 轮 review）
- 三路 trace: opendbrb `docs/reviews/{codex,claude-code-reviewer,go-reviewer}-spec-0.9-{pre,mid,post}-impl-review-2026-05-14.md` (8 trace 全集)

### FROZEN 2026-05-13: spec-0.8-makefile-build — opendbx a7d4a09 + opendbrb 83bb2e0

- [spec-0.8-makefile-build] FROZEN — tag `v0.8.0-stage0.8` (spec-0.7 D-2 dual-repo 自动化)
- 7 deliverable: coverage-gate 四档 tier (core/tool/other/exempt) + bench WARN + 双仓 tag-spec wrapper + release stub + opendbrb top-level Makefile + makefile-check 7 violation kind + 双仓 doc 块
- coverage-gate 91.7% / makefile-check 96.6% (Tool tier ≥90%)
- R2 pre-impl 24 finding + T-13 post-impl 16 finding 全消化
- 三路 trace: opendbrb `docs/reviews/{codex,claude-code-reviewer,go-reviewer}-spec-0.8-post-impl-review-2026-05-12.md`

### 2026-05-13: spec-0.8 T-13 errata — 三路 post-impl review 全消化 (16 finding, 单 PR)

- [spec-0.8-makefile-build] T-13 — codex REQUEST-CHANGES + claude/go APPROVE-WITH-FIXES 共 16 finding 全消化（用户拍板"补 tool 包 ≥90% 独立门槛 + 单 PR 全消化"）
- **CRIT-1 (codex)**: `tools/coverage-gate` + `tools/makefile-check` 从 exempt 移到新 **Tool tier ≥90%**；coverage-gate 91.7% / makefile-check 96.6%
- **HIGH-1 双路 (codex+claude)**: `make gate` 新增 `registry-drift-check` sibling-aware delegation（spec D-5 要求）
- **HIGH (go-reviewer)**: 15 处 `_ := f(...)` test errcheck → `mustParse(t,...)` / `mustCheck(t,...)` helper
- **MED-1 (codex)**: `bench` 移除 `| tee` 改 redirect + cat（rc 真实反映 go test 退出码）
- **MED-2 (codex, opt-in)**: coverage-gate 加 `ListPackages()` + `InjectMissing()` + `-enumerate` flag；默认 off 防 Stage 0 stub 包误伤，全量延 spec-1.X+
- **LOW-1 (codex)**: makefile-check 加 `VHelpTooLong`（spec § 2.3 #4 强制 ≤60 char）；两 Makefile help text 全部缩短
- **LOW-2 (codex)**: `BENCH_OUTPUT ?= bench.out`（对齐 spec D-2）
- 公开类型 godoc 补齐（go-reviewer MED-3/4）
- 三路 trace: opendbrb `docs/reviews/{codex,claude-code-reviewer,go-reviewer}-spec-0.8-post-impl-review-2026-05-12.md`

### Added
- 仓库初始化（README / CLAUDE.md / LICENSE / .gitignore / Makefile）
- `cmd/opendbx/main.go` 骨架（仅 `--version`）
- `.github/workflows/ci.yml` 基础 CI（build + test）
- `go.mod`（Go 1.22+）

### Status
- Stage: 0（工程基建期）
- 设计仓：[sqlrush/opendbrb](https://github.com/sqlrush/opendbrb)（私有）
