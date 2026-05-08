# CLAUDE.md — opendbx 代码仓 AI 协作规则

> **本文件为设计仓 [opendbrb/CLAUDE.md](https://github.com/sqlrush/opendbrb/blob/main/CLAUDE.md) 的副本**。
> 设计仓为权威源，本副本随设计仓更新同步。
>
> 每会话开始 Claude / codex 等 AI 协作者必须读完本文件。

---

## 24 条强制规则

1.  **设计文档权威**：编码必须基于已 approve 的 spec；改设计走 6 步 STOP 流程（STOP → 沟通 → approve → 同步文档 → 修代码 → 验证）。

2.  **5 阶段研发**：spec → review → 代码（TDD）→ CI → review → CI。任何一环缺失都不允许 merge。

3.  **本地 + 远端 CI 双绿**：`make gate` 本地必须先跑通；不允许"本地绿 GitHub 红"重复发生，发现就补到 layer-2 gate 中。

4.  **错误三件套**：所有自定义 error 必含 Code / Message / Hint；禁止 `errors.New("xxx")` 裸字符串。

5.  **无半实现**：要么完整实现（含错误处理 + 测试 + 视图），要么显式拒绝（`return ErrNotImplementedInStageX`）。禁止 `return nil, nil`、空 stub、TODO 不带 spec 引用。

6.  **无第三方依赖随意引入**：spec § 5 未声明的 require 不允许提交；新依赖必须经 spec 决策（含许可证 / 维护状态 / 风险评估）。

7.  **Race detector 必跑**：`go test -race ./...` 0 tolerance；CI 与本地都必跑。

8.  **覆盖率门槛**：核心 package（render / engine / domain/db / app/sentinel）≥ 85%、其他 ≥ 75%、总项目 ≥ 80%。

9.  **性能基线**：每 stage 末 freeze `opendbrb/docs/perf/baseline-stage<N>.json`；CI 自动对比，> 3% WARN、> 5% FAIL。

10. **模型无关**：换 LLM 只改配置不改代码；`llm.Provider` 接口隔离。

11. **诊断三层分离**：探针采集 → 规则陈述事实 → LLM 推理给结论。规则层不下结论，结论必由 LLM 给出。

12. **UI 真终端验证**：所有 UI 改动跑 PTY golden file 测试；headless 编译通过不算。

13. **共用组件改动**：必须列出所有调用方且都 review + 测试。

14. **LLM 链路改动 benchmark**：4 模型 × N 场景 × 3 轮，新版总分 ≥ 旧版 × 0.98。

15. **数据流完整性**：LLM 字段从 SSE 解析到上层使用必须画完整流图。

16. **session 与 audit 分离**：current.jsonl 可覆盖 / audit/ 永久。

17. **工具去重缓存**：同 skill + 参数 hash 在 N 轮内返回 cached。

18. **context budget**：system prompt ≤ 3K tokens、memory ≤ 10 条、history ≤ 20 条、margin ≥ 1K。

19. **多数据库矩阵**：每 skill 必须在所有支持 DB 跑过测试。

20. **Sentinel 可观测性**：每个策略选择必须有解释工具。

21. **commit msg 格式**：conventional commit + footer `Spec: spec-X.Y-*.md`。

22. **版本号**：`v<MAJOR>.<MINOR>.<PATCH>-stage<S>.<N>`；每 spec 完成打 tag。

23. **回答前必须 grep**：架构 / 命名 / 接口问题不凭印象，先扫代码再回答。

24. **文件头与追溯**：新 .go 文件含 `// Author: sqlrush` + design link（引用 spec id）；修改加 `// opendbx: <verb noun>` inline comment。

---

## 不要做的事

- ❌ 复制 opendb 代码到 opendbx（Greenfield 重写，opendb 仅作设计参考与教训来源）
- ❌ 在没有 spec 的情况下开写代码
- ❌ 在 spec 没 approve 的情况下推进 implementation
- ❌ 跳过本地 layer-2 gate 直接 push
- ❌ 用 errors.New / fmt.Errorf 不带 wrap 的方式吞错
- ❌ 引入未经 spec 决策的第三方库
- ❌ 改 UI 只看 headless 测试就 merge
- ❌ 改共用组件不验证全部调用方
- ❌ LLM 链路改动不跑 benchmark 就 merge
- ❌ 覆盖率不达标就 merge
- ❌ 性能基线退化超阈值不 RCA 就 merge

---

## 工作流

```
写 spec  →  review spec（AI + 你）  →  写代码（TDD）  →  本地 gate  →
push  →  GitHub Actions 全绿  →  review 代码（AI + 你）  →
fixup + push  →  CI 二次全绿  →  merge + tag  →  CHANGELOG 更新
```

详见设计仓 [docs/cicd-and-methodology.md](https://github.com/sqlrush/opendbrb/blob/main/docs/cicd-and-methodology.md)。

---

## 修改本规则

修改任何条款必须**先在设计仓 opendbrb/CLAUDE.md 修改 + 走 review 流程**，再同步到本副本。本仓的修改若与设计仓不一致，以设计仓为准。
