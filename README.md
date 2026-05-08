<p align="center">
  <a href="./README.md">English</a> | <a href="./README_zh.md">简体中文</a>
</p>

<h1 align="center">opendbx</h1>

<p align="center">
  <strong>Claude Code-style Agent platform for databases</strong><br>
  <em>Two modes in one binary: Interactive TUI (Claude Code-style) + Autopilot 3-tier cluster (planned)</em>
</p>

<p align="center">
  <a href="https://github.com/sqlrush/opendbx/actions"><img alt="CI" src="https://img.shields.io/github/actions/workflow/status/sqlrush/opendbx/ci.yml?style=flat-square"></a>
  <a href="https://github.com/sqlrush/opendbx/blob/main/LICENSE"><img alt="License" src="https://img.shields.io/badge/license-Apache%202.0-green?style=flat-square"></a>
  <img alt="Go" src="https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat-square&logo=go&logoColor=white">
</p>

> **Status: Stage 0 (engineering bootstrap)** — Stage 0 specs 进行中。
> 设计仓（私有）：[opendbrb](https://github.com/sqlrush/opendbrb) 是项目北极星，包含路线图、specs、AD、CLAUDE.md。
> 当前代码仅为骨架；首个可演示版本预计 Stage 1 末（M5）。

---

## 项目愿景

opendbx 是 opendb 的 Greenfield 重写版本。在保留 opendb 已验证的产品魂（LLM 只做只读查询、变更以"修复方案"形式落入报告由用户手工执行）的基础上：

- **UI 完全对齐 Claude Code**：自研 tcell + Ink-like 渲染引擎，hooks / MCP / Skills / Subagents / Memory / Plan / Output styles / Custom commands / Status line 全套生态
- **删除 Rule Engine**：聚焦 LLM 多轮诊断 + 模型分级降级
- **整合三 agent 特色**：Claude Code / openclaw / hermes 借鉴

12 个月内交付商业版 1.0：PostgreSQL + 完整 CC 风格 TUI + LLM 诊断 + AWR 风格报告 + 单节点 Sentinel。

---

## 路线图

| Stage | 目标 | 周次 |
|---|---|---|
| 0 | 工程基建 + 调研沉淀 | W1-W6 |
| 1 | 渲染引擎 + 基础 TUI + PG MVP | W7-W20 |
| 2 | Claude Code 生态完整接入 | W21-W30 |
| 3 | Sentinel 全量 + 报告强化 | W31-W36 |
| 4 | 商业级加固 | W37-W42 |
| 5 | 1.0 发版 + 商业化 | W43-W48 |
| 6+ | 多 DB / Autopilot / Web 大盘 | post-M12 |

详见设计仓 [development-roadmap.md](https://github.com/sqlrush/opendbrb/blob/main/docs/development-roadmap.md)（私有）。

---

## 编译

```bash
git clone https://github.com/sqlrush/opendbx.git
cd opendbx
make build
./bin/opendbx --version
```

要求 Go 1.22+。

---

## 当前可用命令

骨架阶段，仅有 `--version`。Stage 1 之后会逐步加入 `interact` / `agent` / `cluster` / `admin` 子命令。

---

## 协作规则（CLAUDE.md）

[CLAUDE.md](CLAUDE.md) 为 AI 协作的"宪法"，与设计仓 opendbrb/CLAUDE.md 同步。每次会话开始 Claude / codex 等 AI 协作者必须读完。

24 条强制规则覆盖：spec 优先、TDD、本地+远端 CI 双绿、错误三件套、覆盖率门槛、性能基线、模型无关、诊断三层分离、UI 真终端验证、共用组件改动、LLM benchmark 等。

---

## 仓分工

| 仓 | 内容 | 可见性 |
|---|---|---|
| **opendbrb** | docs / specs / features / roadmap / CLAUDE.md / CHANGELOG | 私有 |
| **opendbx**（本仓） | Go 源码 / Makefile / .github/workflows / scripts / configs | 公开 |

---

## License

Apache License 2.0. 详见 [LICENSE](LICENSE)。
