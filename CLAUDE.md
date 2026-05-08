# CLAUDE.md — opendbx 代码仓 AI 协作规则（精简版）

> **本仓是代码仓**。完整协作规则（4 原则 + 22 规则 + 8 专项 + 12 痛点防御对照表 + pgrac 映射 + 修订协议）维护在**设计仓 [sqlrush/opendbrb](https://github.com/sqlrush/opendbrb)（私有）的 [CLAUDE.md](https://github.com/sqlrush/opendbrb/blob/main/CLAUDE.md)**。
>
> 本文件仅保留代码工作中**立即用得到的核心规则**，AI 加载本文件即可在 opendbx 上正确工作。要追溯规则的完整 Why / 反模式 / Enforcement / 来源，去设计仓查。
>
> 设计仓为权威源；本副本变化时**先在设计仓改 + review，再同步到本仓**。任何冲突以设计仓为准。

---

## § 0 项目定位

opendbx 是 opendb 的 **Greenfield 重写版**，定位为"面向数据库的 Claude Code 风格 Agent 平台"。一个 Go 二进制，两种模态：交互 TUI（CC 风格）+ Autopilot 三层 Agent（Stage 9+）。技术栈：纯 Go + tcell + 自研 Ink-like 渲染引擎。

**协作目标**：Solo + AI 12 个月内交付商业版 1.0（PG only + 完整 CC TUI + LLM 多轮诊断 + AWR 报告 + 单节点 Sentinel）。

---

## § 1 四条核心原则（最高位价值观）

当规则冲突或边缘场景出现时，回到原则做判断。

1. **完整对齐 Claude Code UI 与生态**——视觉 / 交互 / 生态（hooks/MCP/Skills/Subagents/Memory/Plan/OutputStyles/CustomCommands/StatusLine）。这是项目北极星
2. **设计文档权威性**——编码必须基于已 approve 的 spec / AD。发现设计问题走 6 步 STOP 流程：STOP → 沟通 → approve → 更新文档 → 续编码 → CHANGELOG
3. **LLM 不可用拒绝服务**——不实现 Rule Engine（AD-004）。LLM 全失效时返回明确 errcode + Hint，不用规则给出低准确率结论
4. **Solo + AI 现实主义**——少而强、自动化优先。"为完美而扩范围"的需求对照 roadmap 是否可推迟到 Stage 6+

---

## § 2 代码工作时的核心规则（14 条）

### 规则 A1：实现完整性禁止半实现
要么完整（含错误处理 + 测试 + 可观测）要么显式拒绝（`return ErrNotImplementedInStageX` 或 `panic("not implemented")` 或 spec 标注延后）。禁止 `return nil, nil`、空 stub、TODO 不带 spec 引用。

### 规则 A2：错误三件套（Code/Message/Hint）
所有自定义 error 必含 Code（机器可读）/ Message（用户可读 1 句）/ Hint（修复建议 1-2 句）。禁止 `errors.New("xxx")` 裸字符串。错误链必须 `fmt.Errorf("%w", err)` 保留根因。

### 规则 A3：依赖管理（无 spec 决策不允许新依赖）
spec § 5 contract 未声明的 require 不允许提交。stdlib first；新依赖必须经 spec 决策（许可证 / 维护状态 / 替代品 / 风险）。

### 规则 A4：测试分层 + 覆盖率
5 层测试：单元（`*_test.go`）/ 集成（`tests/integration/`）/ E2E（`tests/e2e/`）/ 故障注入（`tests/chaos/`，nightly）/ 性能基线（`tests/perf/`）。单元必测 边界 / 错误路径 / 并发 race / 表驱动。覆盖率：核心 package（render / engine / domain/db / app/sentinel）≥ 85%、其他 ≥ 75%、总项 ≥ 80%。

### 规则 A5：Race detector 必跑
本地 `make gate` 与 CI 必跑 `go test -race ./...`，0 tolerance。任何 race 报告阻塞 merge。

### 规则 A6：本地 + 远端 CI 双绿
任何 commit / PR / CHANGELOG 写"全绿"前必须满足 ① 本地 `make gate` 全过 + ② GitHub Actions ALL JOB 绿。push 后必须 `gh run view` 验证。本地绿 CI 红 → 当下定位为何本地没抓到，并补到 layer-2 gate；不允许"我下次修"。

### 规则 A7：性能基线（每 stage freeze）
每 stage 末跑 `scripts/perf/baseline.sh` → `opendbrb/docs/perf/baseline-stage<N>.json` freeze。CI 自动对比：> 3% WARN / > 5% FAIL 阻塞 merge 触发 RCA。

### 规则 A8：Go 编码规约
- 命名：包小写短词；类型 PascalCase；导出 PascalCase / 私有 camelCase
- 函数：≤ 100 行；嵌套 ≤ 4；early return 优先
- 文件：≤ 600 行（绝对上限 800）；按 feature/domain 组织
- 不可变数据：函数返回新对象不修改输入参数
- 并发：goroutine 必有 cancel/cleanup；channel 同步优先；sync.Once 处理 init-once
- 禁用：`unsafe.Pointer`（除非 spec 批准）、`init()` 副作用、非 main/init 路径的 `panic`

### 规则 A9：代码注释与追溯
- 新 .go 文件含 `// Copyright 2026 opendbx contributors. See LICENSE.` + `// Author: sqlrush` + 包注释引用 spec / AD
- 修改既有文件加 `// opendbx: <verb noun> <context>` inline 注释
- 注释解释 Why 不解释 What
- public API 用 godoc：描述 / 参数 / 返回 / 错误 / Example
- 禁止：TODO 不带 spec id；中文注释（统一英文）

### 规则 A10：Git 工作流（conventional commit）
commit msg 格式 `<type>(<scope>): <subject>\n\n<body>\n\n<footer>`。type ∈ {feat, fix, refactor, test, docs, chore, perf, ci, spec}。footer 必含 `Spec: spec-X.Y-<slug>.md`（hello-world/chore 例外）。提交粒度：1 spec → 1-N 原子 commits。main 保护：禁 force push、禁 amend 已 push、CI 红禁 merge。每 spec 完成 + CI 全绿打 tag `v<MAJOR>.<MINOR>.<PATCH>-stage<S>.<N>`。

### 规则 A11：模型无关（Provider 接口隔离）
换 LLM 模型只改配置不改代码。所有 LLM 调用走 `internal/domain/llm.Provider` 接口；禁止 app 层直接 import 特定 SDK（除 `internal/domain/llm/anthropic` 等 provider 实现外）。

### 规则 A12：诊断三层分离
Layer 1 探针（采集事实，`internal/app/sentinel`）→ Layer 2 规则（陈述事实，**不下结论**，`internal/app/sentinel/rules`）→ Layer 3 LLM（推理 + 方案，`internal/app/diagnose`）。**结论必由 LLM 给**。

### 规则 A13：LLM 链路改动 benchmark
任何 LLM 链路改动（提示词 / function calling schema / context 构造 / streaming / 截断恢复 / 模型切换）必须跑 multi-model benchmark：4 模型 × ≥ 5 场景 × 3 轮，新版总分 ≥ 旧版 × 0.98。落地见 spec-3.11。

### 规则 A14：UI 真终端验证 + 共用组件 + 回答前 grep
- **UI**：所有 UI 改动跑 PTY golden file 测试。真终端 120 列宽手工验证 ≥ 1 次。`runewidth.EastAsianWidth = false`。表格用统一 builder。headless 通过 ≠ UI 通过
- **共用组件**：修改 `render/block/*` / `components/picker.go` / `render/scrollback/*` / `render/streaming/*` 等共用组件前必 grep 列出所有调用方，逐一验证。PR 描述强制列"调用方清单 + 各调用方测试结果"
- **回答前 grep**：架构 / 命名 / 接口 / 数据流问题不凭印象。先 grep 扫代码再回答。发现认知错误立即更新 docs/

---

## § 3 禁忌清单（11 项 ❌）

1. ❌ 复制 opendb 代码到 opendbx（Greenfield 重写，opendb 仅作教训来源；AD-001）
2. ❌ 在没有 spec 的情况下开写代码（违反原则 2）
3. ❌ 在 spec 没 approve 的情况下推进 implementation
4. ❌ 跳过本地 layer-2 gate 直接 push（违反规则 A6）
5. ❌ 用 errors.New / fmt.Errorf 不带 wrap 的方式吞错（违反规则 A2）
6. ❌ 引入未经 spec 决策的第三方库（违反规则 A3）
7. ❌ 改 UI 只看 headless 测试就 merge（违反规则 A14）
8. ❌ 改共用组件不验证全部调用方（违反规则 A14）
9. ❌ LLM 链路改动不跑 benchmark 就 merge（违反规则 A13）
10. ❌ 覆盖率不达标就 merge（违反规则 A4）
11. ❌ 性能基线退化超阈值不 RCA 就 merge（违反规则 A7）

---

## § 4 设计仓索引（要看完整 why / 设计流程 / 教训证据时去这里）

设计仓 [opendbrb](https://github.com/sqlrush/opendbrb)（私有）的对应位置：

| 你想看 | 路径 |
|---|---|
| 完整 22 条规则（含 Why / 反模式 / Enforcement / 来源）| `CLAUDE.md` § 2 |
| 8 个专项规范（渲染引擎 / CC 生态 / LLM 协作 / 数据流完整性 / session-audit 分离 / 工具去重 / 多 DB 矩阵 / Sentinel 可观测性）| `CLAUDE.md` § 3 |
| spec / feature / AD 协作流程 | `CLAUDE.md` § 5 + `docs/cicd-and-methodology.md` |
| spec 13 节模板与深度基线 | `CLAUDE.md` 规则 4 + `docs/methodology/spec-template.md` |
| CI/CD 7 Job Pipeline 详述 | `CLAUDE.md` § 4 + `docs/cicd-and-methodology.md` § 3 |
| opendb 12 痛点防御对照表 | `CLAUDE.md` § 7 附录 A + `docs/surveys/survey-opendb-lessons-learned.md` |
| pgrac 22 规则映射表 | `CLAUDE.md` § 8 附录 B |
| 6 个核心架构决策（AD-001~006） | `docs/architecture-decisions/` |
| 12 月路线图 + spec 状态追踪 | `docs/development-roadmap.md` |
| 项目一页纸总览 | `docs/PROJECT-OVERVIEW.md` |

---

## § 5 修订协议

1. **设计仓 opendbrb/CLAUDE.md 是权威源**。任何规则变更先在设计仓 review + 落地，再同步到本副本
2. 本副本的修改若与设计仓不一致，以设计仓为准
3. 当设计仓有重大改动（新增/删除原则、新增/删除规则、改 4 项核心原则）时，本副本同步更新
4. 任何对本副本的本地修改 PR 必须说明"是否需要先反向同步到设计仓"

### 版本历史

- **2026-05-08 v1.0** 初版精简指针：基于设计仓 CLAUDE.md v1.0（4 原则 + 22 规则 + 8 专项 + 附录 + 修订协议）抽取代码工作时立即用得到的核心规则。
