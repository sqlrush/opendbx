# Changelog — opendbx

按时间倒序记录代码仓的变更。每个 spec 完成 / stage 验收 / release 都追加一行。

格式参考 [Keep a Changelog](https://keepachangelog.com/)。

---

## [Unreleased]

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
