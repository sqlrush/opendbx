# Changelog — opendbx

按时间倒序记录代码仓的变更。每个 spec 完成 / stage 验收 / release 都追加一行。

格式参考 [Keep a Changelog](https://keepachangelog.com/)。

---

## [Unreleased]

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
