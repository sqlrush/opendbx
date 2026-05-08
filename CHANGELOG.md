# Changelog — opendbx

按时间倒序记录代码仓的变更。每个 spec 完成 / stage 验收 / release 都追加一行。

格式参考 [Keep a Changelog](https://keepachangelog.com/)。

---

## [Unreleased]

### Added
- 仓库初始化（README / CLAUDE.md / LICENSE / .gitignore / Makefile）
- `cmd/opendbx/main.go` 骨架（仅 `--version`）
- `.github/workflows/ci.yml` 基础 CI（build + test）
- `go.mod`（Go 1.22+）

### Status
- Stage: 0（工程基建期）
- 设计仓：[sqlrush/opendbrb](https://github.com/sqlrush/opendbrb)（私有）
