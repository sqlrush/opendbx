# CLAUDE.md — opendbx 项目协作规则

> 每会话开始 Claude / codex 等 AI 协作者必须读完本文件。
> 本文件是 opendbx 项目的"宪法"。设计仓 opendbrb 为权威源，代码仓 opendbx 持有同步副本。
> 内容融合 pgrac 22 条规则方法论 + opendb 1 年实战教训 + opendbx 项目特有专项。

---

## § 0 项目定位与协作目标

opendbx 是 opendb 的 **Greenfield 重写版本**，定位为"面向数据库的 Claude Code 风格 Agent 平台"。一个 Go 二进制，两种模态：交互式 TUI（CC 风格）与 Autopilot 三层 Agent（Stage 9+）。技术栈：纯 Go + tcell + 自研 Ink-like 渲染引擎。

**协作目标**：Solo + AI 12 个月内交付商业版 1.0，覆盖 PG + 完整 CC TUI + LLM 多轮诊断 + AWR 风格报告 + 单节点 Sentinel。

**配套文档**（设计仓 opendbrb）：
- `docs/PROJECT-OVERVIEW.md` — 项目一页纸总览
- `docs/architecture.md` — 总体架构
- `docs/development-roadmap.md` — 北极星路线图（每 spec 状态追踪）
- `docs/cicd-and-methodology.md` — CI/CD 与方法论
- `docs/architecture-decisions/AD-001~006.md` — 6 个核心架构决策
- `docs/surveys/` — 三份外部调研归档（CC UI / pgrac 方法论 / opendb 教训）

---

## § 1 四条核心原则（最高位价值观）

四条原则位于规则之上。当规则之间冲突或规则未覆盖的边缘场景出现时，回到原则做判断。

### 原则 1：完整对齐 Claude Code UI 与生态

opendbx UI 与 Claude Code 完全对齐——视觉层 + 交互层 + 生态层（hooks / MCP / Skills / Subagents / Memory / Plan / Output styles / Custom commands / Status line）。这是项目北极星。任何"为了简化而牺牲 CC 对齐"的决策需要走 spec 升 AD 讨论。

### 原则 2：设计文档权威性

所有编码必须基于已 approve 的 spec / AD。发现设计问题时走 6 步流程：**STOP 编码 → 沟通用户 → 用户 approve → 更新文档 → 续编码 → CHANGELOG 记录**。禁止"先写代码，文档后补"或"擅自偏离设计"。

### 原则 3：LLM 不可用拒绝服务（不降级到规则）

opendbx 不实现 Rule Engine（AD-004）。LLM 完全不可用（含本地模型也下线）时，opendbx 返回明确错误码 + Hint，而非用规则给出 30% 准确率的结论。模型分级降级路径（Opus → glm/qwen → 本地模型）已能覆盖大部分场景。

### 原则 4：Solo + AI 现实主义（少而强、自动化优先）

12 月节奏紧。任何"为了完美而扩大范围"的决策需对照 roadmap 是否可推迟到 Stage 6+。优先做的是"可被自动化验证的 spec 颗粒度"——能让 AI 协作者独立完成且 CI 卡住质量。

---

## § 2 22 条强制规则

每条结构：规则正文 / Why / 反模式 / Enforcement / 来源。

### 规则 1：架构决策按类型分时机

**正文**：问题按类型分时机推进。A=范围（包含/不包含的边界）、B=设计（接口与行为契约）、C=参数（具体阈值/默认值）、D=验证（怎么测试通过）。架构级（影响 ≥ 2 个 spec）的问题立即升 spec 升 AD 讨论；不解决不编码。

**Why**：边写边改设计在 Solo+AI 节奏下的代价是回炉重做。pgrac 7 轮收敛历史证明：早讨论比晚改省时间。

**反模式**：把"哪个数据库优先"和"PG 驱动用 pgx 还是 lib/pq"和"连接池默认 10 还是 20"混在一个 spec 里讨论。

**Enforcement**：规则 4 的 SPEC 13 节模板强制 §1 范围分项；spec § 8 Q&A 显示决策点。

**来源**：pgrac 规则 1。

### 规则 2：设计文档权威性（SSOT）

**正文**：所有编码业务逻辑严格基于已归档设计文档（spec / feature / AD / SP）。禁止凭直觉擅自偏离。发现设计问题时按原则 2 走 6 步 STOP 流程。spec frozen 前必读 `docs/methodology/spec-drafting-lessons.md`（spec-0.1 起逐渐积累）。

**Why**：opendb 一年踩过的大坑多数源于"代码与设计脱节"。设计文档是 AI 协作的唯一沟通源。

**反模式**：编码中觉得"这里其实可以这样改更好"就直接改了，下次会话 AI 看代码与 spec 不一致无所适从。

**Enforcement**：code review 必审 spec 关联；commit msg footer 引用 `Spec: spec-X.Y-*.md`；CI Validate 阶段检查 commit msg 含 footer。

**来源**：pgrac 规则 21（核心规则）。

### 规则 3：5 阶段研发流程

**正文**：每个 spec 必走 5 阶段：① 写 spec → ② review spec（AI + 你拍板）→ ③ 写代码（TDD）→ ④ 本地 layer-2 gate + GitHub CI → ⑤ review 代码（AI + 你）→ ⑥ CI 二次绿 → merge + tag。任一环缺失不允许 merge。

**Why**：每环都是质量关卡。spec review 抓设计问题；TDD 抓需求理解；CI 抓自动化能 catch 的；review 抓 CI catch 不到的。

**反模式**：spec 没 approve 就动代码；本地没跑 gate 直接 push；review 还没结束就 merge。

**Enforcement**：CHANGELOG 每 spec 完成行需含 commit hash + Approved 日期 + Frozen 日期；branch protection 要求 review approved。

**来源**：pgrac 规则 6。

### 规则 4：SPEC 13 节深度基线

**正文**：每 spec 必含 13 节、≥ 280 行、≥ 6 Deliverable（含 LOC 估算）、≥ 5 不包含项、≥ 6 风险、≥ 12 DoD、≥ 6 Q&A（每个 ≥ 2 选项 + ★ 推荐 + 理由）、≥ 3 个具体后续 spec id。spec self-review checklist 见 `docs/methodology/spec-template.md`。历史 spec（如 spec-0.X）按本规则达标，前期 hello-world 类 spec 可放宽（在 spec 顶部 status 注明）。

**Why**：spec 太薄 = 决策没挖透 = 实现时返工。pgrac spec-1.2 是参照模板。

**反模式**：spec 只 100 行，§ 6 风险只列 2 条，§ 8 Q&A 写"待补"。

**Enforcement**：spec lint 工具（spec-0.1 落地）检查量化指标；不达标禁止 push。

**来源**：pgrac 规则 22。

### 规则 5：实现完整性禁止半实现

**正文**：要么完整实现（含错误处理 + 测试 + 必要的可观测点），要么显式拒绝（`return ErrNotImplementedInStageX`、`panic("not implemented")`、spec 标注延后 stage）。禁止 `return nil, nil`、空 stub、TODO 不带 spec 引用、永远 dead code。

**Why**：半实现是"看起来通了实际不通"的根源（防 opendb 痛点 1.2 上下文系统接线断裂）。

**反模式**：函数签名占位 `// TODO: implement`、`return nil, nil` 假装成功、stub 让 LLM 误以为有此能力。

**Enforcement**：golangci-lint `nilnil` linter；code review 第 4 项；search "TODO" 必有 spec 引用。

**来源**：pgrac 规则 8 + opendb 痛点 1.2。

### 规则 6：依赖管理（无 spec 决策不允许新依赖）

**正文**：spec § 5 contract 未声明的 require 不允许提交。新依赖必须经 spec 决策（含许可证 / 维护状态 / 替代品 / 风险评估）。stdlib first；优先 minimal surface area 库；禁止 indirect require 不审。已批准依赖列表维护在 `docs/methodology/approved-dependencies.md`。

**Why**：Go 模块图爆炸是供应链风险与构建慢的根源。

**反模式**：随手 `go get` 引入新库，go.sum 多了 30 条 indirect。

**Enforcement**：CI Validate 阶段 `go mod tidy && git diff --exit-code go.mod go.sum`；新 require 必有对应 spec 引用。

**来源**：pgrac 规则 14。

### 规则 7：错误三件套（Code/Message/Hint）

**正文**：所有自定义 error 必含 Code（机器可读，对应错误码注册表）/ Message（用户可读 1 句）/ Hint（修复建议 1-2 句）。禁止 `errors.New("xxx")` 裸字符串。error 链必须 `fmt.Errorf("%w", err)` 保留根因。public API 返回 error 必须是自定义类型（不允许 errors.New 返回）。

**Why**：用户拿到错误时知道"是什么 / 怎么办"。诊断工具的错误消息品质直接影响产品口碑。

**反模式**：`return fmt.Errorf("query failed")` 没上下文、没 Hint。

**Enforcement**：错误码注册表（spec-0.6）；code review 第 3 项；linter 检查 `errors.New` 在 public API 中。

**来源**：pgrac 规则 17 + opendb E4。

### 规则 8：测试分层与覆盖率

**正文**：5 层测试（详见 `docs/cicd-and-methodology.md` § 4）：
- Layer 1 单元（`*_test.go`）：必测 边界 / 错误路径 / 并发 race / 表驱动
- Layer 2 集成（`tests/integration/`）：跨模块 + testcontainers 真 PG + fake LLM
- Layer 3 E2E（`tests/e2e/`）：真 LLM + 真 PG，关键用户旅程
- Layer 4 故障注入（`tests/chaos/`，nightly only）：网络分区 / OOM / 磁盘满 / LLM 空响应
- Layer 5 性能基线（`tests/perf/`）：每 stage freeze baseline.json，CI 自动对比

覆盖率门槛：核心 package（render / engine / domain/db / app/sentinel）≥ 85%、其他 ≥ 75%、总项目 ≥ 80%。

**Why**：单测通过 ≠ 功能通过（防 opendb 痛点 1.2）。每层抓不同问题。

**反模式**：核心 LLM 链路只有单测、UI 改动只 headless 通过。

**Enforcement**：CI 7 job 流水线；codecov 上传 + threshold；race detector 必跑。

**来源**：pgrac 规则 7 + 18 + opendb E15。

### 规则 9：Race detector 必跑

**正文**：本地 `make gate` 与 CI 必跑 `go test -race ./...`，0 tolerance。任何 race 报告阻塞 merge。环境变量 `GORACE=halt_on_error=1` 在 CI 启用。

**Why**：Go goroutine race 是线上 bug 主要来源；race detector 是 catch 这类 bug 的唯一可靠工具。

**反模式**：本地"偶尔不跑 race 因为慢"，几个月后线上崩。

**Enforcement**：Makefile gate 目标 + CI Layer 3 unit test job。

**来源**：pgrac 规则 7 / 12。

### 规则 10：本地 + 远端 CI 双绿

**正文**：任何 commit / PR / spec / CHANGELOG 写"全绿"前必须满足两个条件同时为真：① 本地 `make gate` 全过；② GitHub Actions ALL JOB 绿。push 后必须等 CI 完成并 `gh run view` 验证。本地绿 CI 红 → 必须当下定位为何本地没抓到，并补到 layer-2 gate；不允许"我下次修"。

**Why**：CI 是唯一可信的"全绿"声明源。pgrac 规则 20 是硬约束。

**反模式**：本地绿就 push 走人；CI 红了下次 commit 时再说。

**Enforcement**：branch protection 强制 CI 绿；commit msg 在写"全绿"时附 GitHub Actions run URL。

**来源**：pgrac 规则 20（硬约束）。

### 规则 11：性能基线（每 stage freeze）

**正文**：每 stage 末跑 `scripts/perf/baseline.sh` 生成 `opendbrb/docs/perf/baseline-stage<N>.json`，git commit freeze。CI 每次跑 perf job 与最新 baseline 对比：> 3% 退化 WARN（CHANGELOG 注明）、> 5% FAIL 阻塞 merge 触发 RCA。关键指标见 `docs/cicd-and-methodology.md` § 4 Layer 5。

**Why**：性能不是事后补救的。每 stage 守住基线避免劣化叠加。

**反模式**：Stage 1 不测 perf，到 Stage 4 才发现总体慢了 3 倍无从追溯。

**Enforcement**：CI Layer 5 perf job；`benchstat` 自动对比；FAIL 阻塞 merge。

**来源**：pgrac 规则 7 / 19。

### 规则 12：Go 编码规约

**正文**：
- 命名：包名小写短词（`render` 而非 `RenderEngine`）；类型 PascalCase；导出函数 PascalCase；私有 camelCase；常量 UpperCamelCase 或 ALL_CAPS（视习惯）
- 函数：≤ 100 行；嵌套深度 ≤ 4；early return 优先
- 文件：≤ 600 行（绝对上限 800）；按 feature/domain 组织而非 type；高内聚低耦合
- 不可变数据：函数返回新对象不修改输入参数（除非显式 `Mutate*` 命名）
- 并发：goroutine 必有 cancel/cleanup 路径；channel 同步优先于 sync.Mutex；sync.Once 处理 init-once
- 禁用：`unsafe.Pointer`（除非 spec § 5 显式批准）；`init()` 副作用（init 仅做 register）；`panic` 在非 main / 非 init 路径

**Why**：可读性 / 可测性 / 可维护性。Solo+AI 协作下文件小而专注，AI 上下文负担轻。

**反模式**：800 行的 god struct；10 层嵌套的 if/else；init() 启动连接。

**Enforcement**：golangci-lint `gocyclo`、`funlen`、`nestif`；code review 第 6/7 项。

**来源**：pgrac 规则 12 + opendb E1/E2/E3/E5。

### 规则 13：代码注释与追溯

**正文**：
- 新 .go 文件必含文件头：`// Copyright 2026 opendbx contributors. See LICENSE.` + `// Author: sqlrush` + 包注释引用 spec / AD
- 修改既有文件加 inline `// opendbx: <verb noun> <context>` 注释（如 `// opendbx: handle thinking-mode empty content`）
- 注释解释 Why 不解释 What；只在 Why 不显然时写注释
- public API 用 godoc：含描述、参数、返回、错误、Example
- 禁止：注释里写 TODO 不带 spec id；中文注释（统一英文）

**Why**：3 个月后回看自己的代码也认不得。Author + design link 让追溯成本接近 0。

**反模式**：每行都注释（噪音）；`// TODO: fix this`（哪个 spec？）；中英混写。

**Enforcement**：code review 第 8 项；linter 检查 godoc 缺失。

**来源**：pgrac 规则 11（C 语境改写为 Go）。

### 规则 14：Git 工作流（conventional commit）

**正文**：commit msg 格式 `<type>(<scope>): <subject>\n\n<body>\n\n<footer>`：
- type ∈ {feat, fix, refactor, test, docs, chore, perf, ci, spec}
- scope = 模块名（如 `render/layout`）
- body 解释 Why（不写 What）
- footer 必含 `Spec: spec-X.Y-<slug>.md`（hello-world / chore 类例外）

提交粒度：1 spec → 1-N 原子 commits（每个独立可 revert）；禁止 wip / tmp / typo 堆杂。main 保护：禁 force push、禁 amend 已 push、CI 红禁 merge。tag 时机：每 spec 完成 + CI 全绿立即打 `v<MAJOR>.<MINOR>.<PATCH>-stage<S>.<N>`（参见规则 15）。

**Why**：commit history 是项目第二份文档。conventional commit 让 CHANGELOG 自动化。

**反模式**：commit msg "fix bug"；一个 commit 改 20 个文件；amend 已 push 的 commit。

**Enforcement**：commit-msg hook（spec-0.1 落地）；CI Validate 阶段 commitlint。

**来源**：pgrac 规则 13。

### 规则 15：版本号

**正文**：`v<MAJOR>.<MINOR>.<PATCH>-stage<S>.<N>`：
- MAJOR：1.0 商用发版后启用，breaking change 时 bump
- MINOR：每 stage 完成 bump（v0.1 = stage 0、v0.2 = stage 1 ...）
- PATCH：bugfix bump
- stage<S>.<N>：同 stage 内的 spec 完成序号

每 spec 完成 + CI 全绿 + tag。Stage 验收完成额外打 `v0.<N>.0-stage<N>-accepted` tag。

**Why**：版本号能直接看到当前所在 stage 与 spec。

**反模式**：版本随便打、不打、或与 spec 不对应。

**Enforcement**：spec-0.7 落地自动化 tag 脚本。

**来源**：pgrac 规则 19。

### 规则 16：模型无关（Provider 接口隔离）

**正文**：换 LLM 模型只改配置（`config.yaml` 的 `model.provider` 与 `model.name`）不改代码。所有 LLM 调用走 `internal/domain/llm.Provider` 接口，禁止在 app 层直接 import 特定 SDK（除 `internal/domain/llm/anthropic` 等 provider 实现外）。模型分级（Tier 1 Opus 主力 → Tier 2 glm-5 备选 → Tier 3 deepseek 深度复杂 → Tier 4 本地模型）配置驱动。自动降级在 spec-3.11 落地。

**Why**：LLM 生态半年大变。锁死单一 provider 是技术债。

**反模式**：在 diagnose loop 里 `import "github.com/anthropics/anthropic-sdk-go"`。

**Enforcement**：import linter 限制（spec-0.10）；code review 第 5 项。

**来源**：opendb E6。

### 规则 17：诊断三层分离

**正文**：诊断系统三层严格分离：
- Layer 1 探针（`internal/app/sentinel`）：数据采集（pg_stat_* 查询、指标计算），仅陈述事实
- Layer 2 规则（`internal/app/sentinel/rules`）：分类与陈述（如"逻辑读 30min 内增长 30%"），不下结论
- Layer 3 LLM（`internal/app/diagnose`）：基于 Layer 1/2 输出做推理与方案，结论必由 LLM 给

规则层禁止下结论（不允许"这是慢 SQL 问题"），结论必由 LLM 给。

**Why**：opendb 已验证有效：规则纯陈述事实可维护，LLM 推理负责理解；混在一起两边都不专。

**反模式**：规则里写"if cpu > 80 then return '系统过载，建议添加索引'"。

**Enforcement**：code review；专项 § 3.4 数据流完整性规范。

**来源**：opendb E7。

### 规则 18：LLM 链路改动 benchmark

**正文**：任何 LLM 链路改动（提示词 / function calling schema / context 构造 / streaming 处理 / 截断恢复 / 模型切换）必须跑 multi-model benchmark：4 模型 × ≥ 5 场景 × 3 轮，新版总分 ≥ 旧版 × 0.98。评测维度：准确性 30% / 工具效率 20% / 可执行性 20% / 结构化质量 15% / 信息完整性 15%。Benchmark 框架在 spec-3.11 落地。

**Why**：LLM 改动间歇性故障极难排查（防 opendb 痛点 1.5）。Multi-model 抓 provider 行为漂移。

**反模式**：改完 prompt 单测过了就 merge，结果某个模型某种场景下回归 30%。

**Enforcement**：CI Layer 5 perf job 包含 benchmark；CHANGELOG 写明对比结果。

**来源**：opendb E14（提升为强制规则）。

### 规则 19：Context budget

**正文**：LLM context 严格预算：
- system prompt ≤ 3K tokens（含 PROFILE.md 注入；Prompt Cache 缓存）
- memory 条目 ≤ 10 条（按 mtime 排序，旧的归档）
- session history ≤ 20 条消息（超出 compaction）
- 单轮工具超时 30s；总诊断超时 10min；触发警告在最后 1min
- margin ≥ 1K tokens（不用满 context limit）

预算超限的处理：① 主动截断 memory；② 强制 `/session new`；③ 不静默吞错——必给出明确错误码 + Hint。

**Why**：context 超限是不可恢复故障源（防 opendb 痛点 1.8）。

**反模式**：拼上下文不算 token，靠 LLM 自己截断恢复。

**Enforcement**：spec-3.10；context 构造时 token 计数器；超限测试用例。

**来源**：opendb 痛点 1.8 + 规则化。

### 规则 20：UI 真终端验证

**正文**：所有 UI 改动必须跑 PTY golden file 测试（`tests/uitest/`，spec-0.11 落地、spec-4.13 完整化）：
- 真终端宽度 120 列验证
- East Asian Ambiguous 字符宽度禁用（`runewidth.EastAsianWidth = false`）
- 表格用统一 builder（`internal/app/cli/components/table.go`）
- 改 UI → 跑测试 → 对比 golden file → 终端手工验证（≥ 一次）→ merge

headless 编译通过 ≠ UI 通过。

**Why**：UI 表格换行溢出 / 边框对齐错位是 opendb 反复踩的坑（痛点 1.11）。

**反模式**：改了渲染只看 `go test ./internal/app/cli/...` 通过就 merge。

**Enforcement**：CI E2E job 含 PTY 测试；不允许跳过。

**来源**：opendb E12。

### 规则 21：共用组件改动列出全调用方

**正文**：修改共用组件（`render/block/*`、`components/picker.go`、`components/table.go`、`render/scrollback/*`、`render/streaming/*` 等）前必须列出所有调用方（`grep -r "ComponentName"`），逐一验证。PR 描述强制列出"调用方清单 + 各调用方测试结果"。

**Why**：opendb 痛点 1.3 picker.go 修改影响 `/login`、`/model`、`/llm` 三命令、一周 6 用户反馈。

**反模式**：改 picker 只测 `/login`，没看 `/model` 和 `/llm`。

**Enforcement**：code review 第 11 项；PR template 强制清单字段。

**来源**：opendb E12 + 痛点 1.3。

### 规则 22：回答前必须 grep

**正文**：架构 / 命名 / 接口 / 数据流问题不凭印象回答。先 grep 扫代码再回答。发现认知错误立即更新 docs/。AI 协作者每次会话开始必读 `MEMORY.md`（如有）并执行该规则。

**Why**：opendb 一年踩坑大半源于 AI 凭印象回答导致设计偏差。

**反模式**："我记得 picker.go 是这样的"——直接动手改。

**Enforcement**：自律。但 spec-2.10 memory 系统会将"被发现凭印象"作为反例沉淀。

**来源**：opendb 已验证规则。

---

## § 3 专项规范

### § 3.1 渲染引擎规范（tcell + 自研引擎，AD-002 落地）

opendbx 渲染层是项目最重的子系统（~8-10K LOC），有以下特别约束：

- **不引入 bubbletea / lipgloss / ratatui**：所有渲染 primitive 自研。spec-0.10 lint 配置 import deny list 阻止意外引入
- **render 子包分层不可串扰**：terminal → buffer → layout → optimizer → scheduler → scrollback → streaming → block → style → width 的依赖方向严格单向（向下）。CI 用 `goda` 工具（spec-0.10）验证依赖图无环
- **每 block 类型一个文件**（非一个文件多 block）：`render/block/{message,toolcall,compact,markdown,code,diff,...}.go` 各自含 RenderNode interface 实现 + table-driven 单测
- **改渲染必跑 PTY 测试 + 27k 消息长会话压测**（spec-1.5 验收用例之一）
- **流式 token 不重排**（防痛点 1.1 类比）：已完成行不动，只追加末行；半截 markdown 按 `formatToken` 模式渲染未闭合 fence
- **不实现 React useDeferredValue 完整等价**：用 goroutine pool + Bubbletea-like Cmd tick 切片渲染（拿到 80% 体验）；超出 80% 体验范围在 spec § 6 风险登记
- **字符宽度统一**：所有 stringWidth 调用走 `render/width.Width()` 包装；不允许直接调 `runewidth.StringWidth`

### § 3.2 Claude Code 生态对齐规范（AD-005 落地）

- **hooks 事件契约**（PreToolUse / PostToolUse / SessionStart / SessionEnd / UserPromptSubmit）：JSON schema 与 Claude Code 1:1 对齐；扩展事件用 `opendbx_*` 前缀
- **MCP server / client 协议**：完全遵循 Anthropic MCP 规范；opendbx 自身作为 MCP server 时暴露的 DB 工具命名 `opendbx_db_*`、`opendbx_diag_*`
- **SKILL.md 格式**：frontmatter 字段与 CC 兼容（`name`, `description`, `allowed-tools`），扩展字段用 `opendbx_*` 前缀
- **slash 命令冲突处理**：与 CC 同名命令（`/help`, `/clear` 等）保持兼容语义；不冲突的扩展用 `/db-*` 前缀
- **Plan / Todo / Memory / Subagent**：行为完全对齐 CC，差异点必须在 spec 明确说明并升 AD
- **CC 内部未发布功能（KAIROS / DAEMON / BRIDGE / PROACTIVE 等 108 个）禁止照抄**——这些是 Anthropic 内部实验，不应跟进

### § 3.3 LLM 协作专项

涵盖提示词 / function calling / 模型分级 / cost / cache 五大块：

- **三层 system prompt**：通用基础规则 → 库特定规则 → 用户自定义策略；MEMORY/PROFILE 末尾注入（spec-2.10/2.11）
- **工具描述用决策树**：每个工具 description 写"何时用 / 何时不用"，不写用法手册（spec-2.1）
- **function calling 工具粒度**：细粒度（如 `/topsql` 而非 `/sql`），借鉴 opendb 90+ skill 经验，先精选 20-30 核心
- **多轮诊断终止**：单轮工具 30s + 总诊断 10min（spec-1.21）+ memory_update 工具让 LLM 主动判断收敛
- **模型分级**：Tier 1 Opus 主力 / Tier 2 glm-5 备选 / Tier 3 deepseek 深度 / Tier 4 本地。配置驱动，自动降级（spec-3.11 + spec-1.20）
- **Prompt Cache**：system prompt 全量缓存 + memory 增量缓存（spec-1.20 + spec-3.10）
- **Cost tracker + 熔断**：token 累计跟踪 + 速率限制告警 + 上限熔断（spec-3.8）
- **Upstream proxy**：企业部署的 LLM 代理（审计 / 限流 / 地域路由）（spec-3.9）
- **Constrained generation**：小模型用 JSON schema 输出（spec-3.11）

### § 3.4 数据流完整性规范（防痛点 1.1）

任何涉及 LLM SSE 解析 → engine 字段传递 → 上层使用的链路改动，必须：

1. 在 spec 中画完整数据流图（mermaid 或 ASCII 图），从 SSE 到顶层使用每个节点列出
2. 每个节点 +1 单测验证字段从入到出存在性
3. 必须 E2E 测试覆盖（fake provider 强制 `finish_reason=length`，断言 `resp.Truncated=true` 且恢复路径执行）
4. 同字段多产出路径（如 streaming + non-streaming），全路径覆盖

### § 3.5 session 与 audit 分离规范（防痛点 1.4）

session 文件分两层：
- `~/.opendbx/sessions/<instance>/current.jsonl`：当前会话，drift 触发可清空，可覆盖
- `~/.opendbx/sessions/<instance>/audit/<timestamp>.jsonl`：永久归档，故障取证用

drift 机制（如保留）只影响内存加载（historyMessages = nil），不影响文件持久化。提供 `opendbx /session recover <id>` 工具恢复早期 session。具体落地见 spec-2.12 / 2.13。

### § 3.6 工具去重缓存规范（防痛点 1.7）

LLM 在 N 轮内对同一 skill + 相同参数 hash 的调用必须返回 cached（默认 N=3）。cache 命中时返回结果 + 提示 LLM"已有此结果"，让 LLM 基于结果继续分析。落地：spec-1.22。

### § 3.7 多 DB 矩阵规范

虽然 12 月 MVP 仅 PG，但 `internal/domain/db/` 接口设计必须为多 DB 预留：
- `Driver` interface 抽象出最小公共集
- 每个 skill 在 description 标注"支持的数据库列表"
- skill schema 一旦定义就冻结，新增字段而非重命名（spec-2.4）
- Stage 6 引入 MySQL 时不允许"先 MySQL stub 待补"

### § 3.8 Sentinel 可观测性规范（防痛点 1.10）

每个 sentinel 检测策略选择必须有解释工具：

- `opendbx /debug sentinel-status`：当前状态、每指标当前值 vs baseline vs 阈值、最后触发的指标 + 策略 + 时间
- `opendbx /debug sentinel-explain <metric>`：该指标为何选此策略、阈值如何计算、SoftAbsoluteMin 与 HardCeiling
- 每个策略（T1-T9）有对应 chaos engineering 测试场景

落地：spec-3.6。

---

## § 4 CI/CD 7 Job Pipeline 详述

完整 pipeline 设计见 `docs/cicd-and-methodology.md` § 3。本节强调与本 CLAUDE.md 规则的强约束关系：

| Job | 执行内容 | 关联规则 |
|---|---|---|
| 1. Validate | gofmt / vet / commit-msg / go mod tidy | 规则 6, 13, 14 |
| 2. Build | `go build` matrix [linux, darwin] × [amd64, arm64] | 规则 12 |
| 3. Unit Test | `go test -race -cover` ≥ 80% gate | 规则 8, 9 |
| 4. Integration | testcontainers + fake LLM provider + 真 PG | 规则 8, 17 |
| 5. E2E | 真 LLM + 真 PG + PTY UI 测试 | 规则 8, 18, 20 |
| 6. Chaos (nightly) | 故障注入（network / disk / OOM / LLM 空响应） | 规则 8 |
| 7. Perf/Bench | 对比 baseline；> 3% WARN / > 5% FAIL | 规则 11, 18 |
| 8. Security | gosec / trivy / govulncheck | 规则 6 |

**Fail-Fast 策略**：1/2/3 红则 4-8 取消，节省 minutes。

**触发**：push to main / PR / manual / nightly cron 2AM。PR 跑 1-5 + 7 + 8（~9 min wall）；Nightly 跑全集。

---

## § 5 spec 生命周期与文档同步

```
1. 写 spec       → opendbrb/specs/stage-X/spec-X.Y-*.md（DRAFT 标记，13 节，280+ 行）
2. AI review 多轮 → claude / codex 各跑一轮，发现 placeholder/矛盾/范围/风险，inline 修
3. 你 approve     → 删 DRAFT 标 + 加 Approved YYYY-MM-DD；CHANGELOG 加一行
4. 写测试        → opendbx/...; TDD 先于实现
5. 写实现        → opendbx/...; 满足 spec、覆盖率达标
6. 自审 + AI review → code-reviewer + go-reviewer + security-reviewer 三个 agent
7. 本地 gate 全绿 → make gate
8. push + GitHub CI 全绿 → 验证 gh run view
9. 你 review 代码  → 发现问题回 step 5
10. fixup + push → CI 二次绿
11. merge + tag   → v0.X.Y-stage<S>.<N>
12. spec FROZEN  → 加 FROZEN <commit-hash>
13. CHANGELOG / roadmap 状态同步 → roadmap 表中对应行 ⬜ → ✅，补充交付物（含 tag）
```

**roadmap 状态同步是强制项**（roadmap § 12）：spec 完成必须更新 `docs/development-roadmap.md` 中对应行的状态与交付物。spec-0.16 stage0-acceptance 落地 CI 检查"已完成的 spec 必须在 roadmap 中标 ✅"。

---

## § 6 禁忌清单（11 项 ❌）

1. ❌ 复制 opendb 代码到 opendbx（Greenfield 重写，opendb 仅作设计参考与教训来源；AD-001）
2. ❌ 在没有 spec 的情况下开写代码（违反原则 2 / 规则 3）
3. ❌ 在 spec 没 approve 的情况下推进 implementation
4. ❌ 跳过本地 layer-2 gate 直接 push（违反规则 10）
5. ❌ 用 errors.New / fmt.Errorf 不带 wrap 的方式吞错（违反规则 7）
6. ❌ 引入未经 spec 决策的第三方库（违反规则 6）
7. ❌ 改 UI 只看 headless 测试就 merge（违反规则 20）
8. ❌ 改共用组件不验证全部调用方（违反规则 21）
9. ❌ LLM 链路改动不跑 benchmark 就 merge（违反规则 18）
10. ❌ 覆盖率不达标就 merge（违反规则 8）
11. ❌ 性能基线退化超阈值不 RCA 就 merge（违反规则 11）

---

## § 7 附录 A：opendb 12 痛点防御对照表

来源：`docs/surveys/survey-opendb-lessons-learned.md`。每个痛点对应 opendbx 的防御 spec / 规则。

| # | opendb 痛点 | 根因 | opendbx 防御 spec / 规则 |
|---|---|---|---|
| 1.1 | 流式截断恢复失效（P0） | SSE 解析检测到 finish_reason 但漏写字段；多路径只修一条；缺 E2E | spec-1.6 + spec-1.21；规则 8 + § 3.4；CLAUDE 规则 15 已废弃改用本表 |
| 1.2 | 上下文系统接线断裂（P0） | 编译通过 ≠ 功能通过；DiagnoseSkill 每次 new；单测覆盖单模块 | 规则 5（无半实现）；规则 8 Layer 2/3；spec-2.10 集成测试必须验证完整调用链 |
| 1.3 | UI 共用组件级联崩溃 | picker 修改影响 /login, /model, /llm | 规则 21（共用组件改动列全调用方）+ 规则 20（UI 真终端验证） |
| 1.4 | drift + session 覆盖复合副作用 | drift 阈值低 + Jaccard 算法 + saveSession 全文件覆盖 | § 3.5 session/audit 分离；spec-2.12 / 2.13 |
| 1.5 | LLM 诊断输出空白（思考模式不一致） | thinking 模式偶发空 content；engine 不调 OnStream | spec-1.20 强制处理 thinking 字段；规则 7 错误三件套必有 Code+Hint |
| 1.6 | 规则引擎手写反模式（已废弃） | 手写规则维护困难 | AD-004 删除 Rule Engine |
| 1.7 | 多轮诊断中工具重复调用 | LLM 缺乏"已调过"记忆；长链 5+ 轮易现 | § 3.6 工具去重缓存；spec-1.22 |
| 1.8 | 上下文超限自动恢复脆弱 | input token 预测不准；二阶截断 | 规则 19 context budget；spec-3.10；spec-1.20 Prompt Cache |
| 1.9 | 模型输出不稳定导致评测难 | 小模型同 prompt 两次差 30%+ | 规则 18 multi-model benchmark × 3 轮；spec-3.11 |
| 1.10 | Sentinel 探针策略选择隐蔽错误 | 策略选择规则隐含；错策略导致延迟 | § 3.8 sentinel 可观测性；spec-3.3 / 3.6 |
| 1.11 | UI 表格换行溢出 | East Asian Ambiguous 字符；termwidth 与 lipgloss 不一致 | 规则 20 + spec-0.11 / spec-4.13 PTY golden file；统一表格 builder |
| 1.12 | 跨 DB SQL 差异隐蔽 | 集成测试没覆盖所有 DB | § 3.7 多 DB 矩阵；spec-2.4 schema 冻结；MVP 期仅 PG 但接口预留 |

---

## § 8 附录 B：pgrac 22 条规则映射表

| pgrac 规则 | opendbx 规则 | 备注 |
|---|---|---|
| 1 未决问题三铁律 | 规则 1 | 改写为"按类型分时机" |
| 2 特性归档必走流程 | 规则 4（spec 13 节） | 合并到 spec 模板 |
| 3 Oracle 诚实性 | — | opendbx 不对标 Oracle，舍弃 |
| 4 可观测性设计纪律 | § 3.8 | 改写为 sentinel 可观测性 |
| 5 系统级 TODO（错误码 / 进程） | spec-0.6 / 错误码注册表 | |
| 6 5 阶段研发流程 | 规则 3 | 直接采纳 |
| 7 测试与质量 | 规则 8 + 9 + 11 | 拆 3 条 |
| 8 实现完整性 | 规则 5 | 直接采纳 |
| 9 开发节奏（Stage） | roadmap § 12 | 移到 roadmap |
| 10 设计盲区三同步 | 规则 2 子项 | 合并到原则 2 |
| 11 代码注释规范 | 规则 13 | C 改写为 Go |
| 12 C 编程规约 | 规则 12 | C 改写为 Go |
| 13 Git 工作流 | 规则 14 | 直接采纳 |
| 14 依赖管理 | 规则 6 | 直接采纳 |
| 15 安全规约 | spec-4.1~4.4 | 移到 Stage 4 spec |
| 16 PG 特有编程约束 | — | C/PG 特有，舍弃 |
| 17 可观测性 / debug 标准 | 规则 7（错误三件套）+ § 3.8 | |
| 18 测试组织 | 规则 8 | C 改写为 Go |
| 19 版本与 release 节奏 | 规则 15 | 直接采纳 |
| 20 CI/CD 双流程 | 规则 10 | 直接采纳（pgrac 硬约束） |
| 21 设计文档权威性 | 原则 2 + 规则 2 | 提升为原则 |
| 22 SPEC 详细度 | 规则 4 | 直接采纳 |

opendbx 新增规则（pgrac 没有）：
- 规则 16 模型无关（来自 opendb E6）
- 规则 17 诊断三层分离（来自 opendb E7）
- 规则 18 LLM 链路 benchmark（来自 opendb E14）
- 规则 19 Context budget（来自 opendb 痛点 1.8）
- 规则 20 UI 真终端验证（来自 opendb E12）
- 规则 21 共用组件改动列全调用方（来自 opendb 痛点 1.3）
- 规则 22 回答前必须 grep（来自 opendb 已验证）

---

## § 9 修订协议

任何对本文件的修改必须：

1. PR 描述写明：改动条款编号 + 改动理由 + 涉及到的 spec / opendb 教训证据 / 影响范围
2. 跑一轮 AI review（claude / codex 各一轮）+ 你拍板
3. CHANGELOG 加一行记录条款变更（含改动前后对比的简述）
4. 同步代码仓 opendbx/CLAUDE.md（设计仓为权威源，代码仓副本随之更新；可由 CI 自动同步，spec-0.1 决定）
5. 重大修改（如新增/删除原则、新增/删除规则、改 4 项核心原则）需在 CHANGELOG 标 BREAKING

### 版本历史

- **2026-05-08 v1.0 初版**：基于 brainstorming § 1-§ 4 + pgrac 22 条规则方法论 + opendb 1 年实战教训合成。4 条原则 + 22 条规则 + 8 个专项规范 + 12 痛点防御对照 + pgrac 映射表。
