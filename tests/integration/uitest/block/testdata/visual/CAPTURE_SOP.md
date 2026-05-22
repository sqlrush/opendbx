# spec-1.7 T-2.5 CC fixture capture SOP

> spec-1.7-block-interface.md T-2.5 CC fixture live capture 流程.
>
> 目的: 抓真实 Claude Code session 的 Message block 视觉 fixture, 作为
> spec-1.7 R2 D5/D7/D6 (HIGH-H/M-4/HIGH-K) golden file 基线. 用户北极星
> "CC UI 100% 还原" 起点 — placeholder 风险高, 真 fixture lock-in 前不
> 实施 Q3-Q6 visual constants (spec-1.7 R2 D5 决策).

## 范围 (R2 D7 M-5: 7 fixtures)

| # | TestName | 输入 prompt | 期望 Message 形态 |
|---|---|---|---|
| 1 | `MessagePlain` | "Say hello world once and stop." | 单行 plain text |
| 2 | `MessageMultiline` | "Write 3 lines: line A, line B, line C. One per line." | 3 行 plain text |
| 3 | `MessageMixedProseFence` | "Explain Go in 1 sentence, then show 'fmt.Println' as a Go code fence with package declaration." | intro prose + ```go fence + (可能 outro) — **R-12 mixed prose+fence 关键 fixture** |
| 4 | `MessageCodeFenceGo` | "Show only a Go code fence with: package main; func main(){}; no prose." | 仅 ```go fence (无 intro/outro) |
| 5 | `MessageTruncated` | "Repeat the word 'token' 200 times." + 设 max_tokens=50 (或 CTRL-C 半截; 触发 length truncation) | Truncated 末尾 "…" |
| 6 | `MessageContinued` | (难直接触发; capture 时 caller wire stream.AppendChunk → lineBuf cap overflow → Continued=true) — **若 CC 无直接触发, mark deferred**, 用 mock fixture | 末尾 "…" + StyleDimmed (区分 Truncated StyleWarning) |
| 7 | `MessageEmpty` | (难直接触发 — thinking-only; 触发需 stream 仅 think 不 emit content) — **若 CC 无直接触发, mark deferred**, 用 mock fixture | "(no output)" placeholder |

## Capture 步骤

### 通用 setup

1. 终端: iTerm2 (主); 字号 14pt monospace; Theme: Claude Code default; 窗宽 120 cols × 40 rows (可宽; render 后 crop)
2. `claude --version` 记录 (写入 metadata sidecar `cc_version` 字段)
3. 终端 cols/rows + theme + font 全 metadata sidecar 记录
4. 关闭其他后台 progress/notification (防截屏 contamination)

### 每 fixture step

```bash
# 1. 启 CC session (干净 instance)
claude

# 2. 输入 prompt (见上表)

# 3. CC 响应渲完后, 截 ANSI:
#    iTerm2 → Edit → Find / Select All → Copy with Styles (得 ANSI)
#    或 tmux capture-pane -e -p > /tmp/cc-MessagePlain.ansi
#    或 用 asciinema record + extract

# 4. 截 PNG (golden):
#    iTerm2 → cmd+shift+s screenshot 选区域 (仅 Message block)
#    或 freeze 工具: freeze --input /tmp/cc-MessagePlain.ansi \
#        --output testdata/visual/MessagePlain/golden.png

# 5. 保存到 fixture 目录:
mkdir -p internal/app/cli/render/block/testdata/visual/MessagePlain
mv /tmp/cc-MessagePlain.ansi   internal/app/cli/render/block/testdata/visual/MessagePlain/input.ansi
mv /path/to/screenshot.png     internal/app/cli/render/block/testdata/visual/MessagePlain/golden.png

# 6. 写 metadata sidecar:
cat > internal/app/cli/render/block/testdata/visual/MessagePlain/metadata.json <<EOF
{
  "fixture":         "MessagePlain",
  "captured_at":     "2026-05-22T...",
  "cc_version":      "claude-code 1.x.x",
  "terminal":        "iTerm2 3.5.x",
  "cols":            120,
  "rows":            40,
  "font":            "SF Mono 14pt",
  "theme":           "Claude Code default",
  "prompt":          "Say hello world once and stop.",
  "sanitizer":       "v1 — no PII, no timestamps in body",
  "notes":           ""
}
EOF

# 7. 验证 golden 视觉对齐 (人工 review screenshot)
```

### Sanitizer (R2 D7 M-4 governance)

- **必删**: 用户 prompt 中任何 私人/敏感信息 (替换为 generic prompt 上表)
- **必删**: 输出中任何 timestamp / session id / user name
- **保留**: pure visual rendering (text content + style/color)
- 若 fixture 含意外 私人内容, 直接 abort + 重新 capture

### Update protocol

- CC visual change (e.g. Anthropic 发新版 改了 marker rune / lang label style) 时:
  1. 重新按本 SOP capture CC fixture (`golden.png` + `input.ansi` + metadata)
  2. 跑 `BLOCK_VISUAL_REQUIRED=1 make ui-block-golden` 验证 opendbx 输出对齐新 fixture
  3. 用户视觉 review 新 golden 是否 acceptable
  4. 若 acceptable: commit; 若不: spec-1.7 errata 改 const 与 CC 对齐
- Quarterly review: 用户跑全 7 fixtures 重 capture 1 次, drift check

## 后续 lock-in (spec-1.7 R2 D5)

7 fixtures capture 完毕后, 我会基于 fixture 视觉:

- Q3 lang label 真 visual (e.g. `─── go ───` vs `▌go` vs 无 label) → final rune + Style
- Q4 Continued marker rune (现 placeholder "…" + StyleDimmed) → 真 CC visual confirm
- Q5 Truncated marker rune (现 placeholder "…" + StyleWarning) → 真 CC visual confirm
- Q6 Empty placeholder text (现 "(no output)") → 真 CC visual confirm

如 #5/#6/#7 无法直接 CC 触发, mark `deferred: mock` 在 metadata, spec-1.7 R3 review 时决定真触发 vs 维持 mock.

## 半天卡退路径 (用户拍板)

若 T-2.5 半天内卡住 (e.g. CC 无法触发 Truncated/Continued/Empty), 退回路径 B:
- 用 R2 placeholder ("…" + StyleDimmed/Warning + "(no output)")
- T-3 实施推进
- T-9 用 mock fixture 测; **R3 errata 用 fixture verify gate blocking T-10**

---

## spec-1.9 ToolUse fixture extension (2026-05-22 加)

> spec-1.9-toolcall-block.md APPROVED 2026-05-22 → R2.1.3 D-9 7 fixture 需同 session 捕获 (用户拍板: 14 fixture/session 复用 1 次 CC capture 防 spec-1.7 / 1.9 基线不一致).
>
> **重要**: spec-1.9 impl 在 spec-1.7 merge 后才开新 PR; 本节 ToolUse fixtures **捕获后 parked**, 待 spec-1.9 impl T-8 创建 `ui-tooluse-golden` Makefile target + `tests/integration/uitest/block/tooluse_render_test.go` 消费. 当前 ui-block-golden 仅消费 Message fixtures.

### 范围 (spec-1.9 R2.1.3 D-9 fixture 列表)

| # | TestName | 触发场景 (CC prompt + 上下文) | 期望 ToolUse 形态 (per CC source 验证) |
|---|---|---|---|
| 1 | `ToolUseQueued` | 触发多个 Bash 调用，第 2 个 Bash 处 queued 状态时截屏 (CC 默认 1 个 Bash 并发); 例: `运行: pwd && ls && date && echo done` (CC 串行多 bash) | Bash header (`pwd` etc.) + dim `Waiting…` 二行 (BashTool/UI.tsx:154-158) |
| 2 | `ToolUseRunningGeneric` | 触发未注册 tool name (即 generic fallback 路径). MVP 无好触发点 — **deferred: mock**, 用 placeholder header `Foo(key=val)` mock fixture | Generic compact `Name(key=val, key2=val2)` ≤ cols/2 |
| 3 | `ToolUseRunningBash` | `跑这命令: sleep 3 && echo done` — sleep 期间 ToolUse 处 Running (无 progress), 立即截屏 (3 秒窗口) | Bash header (`sleep 3 && echo done` 或截断) + dim `Running…` 二行 (BashTool/UI.tsx:148) |
| 4 | `ToolUseRunningRead` | `读 main.go 第 1-100 行` (触发 Read tool). Read 通常 instant, 难截 Running 态 — **若不易捕**, 截 Resolved 后 mark in metadata `state_at_capture: resolved-not-running`; 或用 `读 /Users/sqlrush/very/large/file.txt` 慢 IO | Read header `path · lines 1-100` (FileReadTool/UI.tsx:30-65); 无 progress (Read 不 implement ProgressRenderer) |
| 5 | `ToolUseWaitingPermission` | 配置 CC permission mode 非 auto (需 user 确认 Bash); 触发 Bash 然后**不点确认**, 截屏 permission 等待态. 例 prompt: `运行: rm /tmp/test-permission` (敏感命令必弹 permission) | Bash header (`rm /tmp/test-permission`) + dim 二行 `Waiting for permission…` (AssistantToolUseMessage.tsx:240) |
| 6 | `ToolUseRunningBashWithProgress` | `运行: for i in $(seq 1 100); do echo line $i; sleep 0.05; done` (5 秒 100 行 progress); progress 滚动期间截屏 | Bash header + ShellProgressMessage block (elapsedSeconds / totalLines / totalBytes; BashTool/UI.tsx:131-153) |
| 7 | `ToolUseResolvedRead` | `读 main.go 第 1-100 行` — Read 完成后保留状态 (Resolved) 截屏 | Read header `path · lines 1-100` 单行 (无 progress); AssistantToolUseMessage.tsx:103 `lookups.resolvedToolUseIDs.has(param.id)` 路径 |

### 时序敏感 fixture (#1 / #3 / #5 / #6) capture 提示

- `ToolUseQueued`: CC 串行多 Bash; 同时 trigger 多个 bash tool calls 然后立即截屏 (queued window < 1s)
- `ToolUseRunningBash` / `RunningBashWithProgress`: 用 `sleep N` / 长 loop 制造 N 秒 capture 窗口
- `ToolUseWaitingPermission`: CC 必须不在 auto-approve mode (`/permission` 检查); 用 sensitive 命令触发 prompt 不点确认
- 建议: 每个 transient state 录 asciinema 全程 + 事后挑帧 / `script -t` 时间戳 replay

### 路径 & 文件结构 (与 spec-1.7 一致)

```
tests/integration/uitest/block/testdata/visual/<ToolUseTestName>/
├── golden.png       (PNG screenshot of just the ToolUse block region)
├── input.ansi       (ANSI escape capture, sanitized)
└── metadata.json    (cc_version / terminal / cols / rows / prompt / sanitizer / notes)
```

每 metadata.json 加 spec-1.9 特有字段 `tool_use_state` ∈ {`queued`, `running`, `waiting_permission`, `resolved`} + `adapter` ∈ {`bash`, `read`, `generic`} for tooluse_render_test.go consumer 时筛选.

### Parking 状态 + 解锁路径

| 状态 | 含义 |
|---|---|
| **本 session capture** | spec-1.7 Message 7 + spec-1.9 ToolUse 7 = 14 fixture 一次抓 |
| **spec-1.7 PR #54 merge** | Message 7 fixture 立 wire 进 `make ui-block-golden`; ToolUse 7 fixture **parked** in testdata (CI 不消费, no harness yet) |
| **spec-1.9 impl PR T-8** | 创建 `tooluse_render_test.go` + `ui-tooluse-golden` Makefile target + ci.yml step; 解锁 ToolUse fixture CI 消费; **BLOCK_VISUAL_REQUIRED=1 make ui-tooluse-golden** 跑通 = T-8 DoD |
| **spec-1.9 T-10 FROZEN** | 14 fixture 全 CI gate; 任何 CC visual drift 触发 spec-1.7a / 1.9a errata |

### ToolUse fixture lock-in 决策 (本 capture session 后)

- D-7 stateIndicator: per CC source, 用 `BLACK_CIRCLE` / `ToolUseLoader` / `MessageResponse` composition. fixture 后 lock 真实 rune/text/style.
- Q3 indicator 矩阵: A. fixture-derived (★A R2.1.3 MED-2). Capture 后 finalize.
- Q5 fixture list: 7 fixtures **R2.1.3 锁定**, 不再扩张 (10 fixture 边际收益低).
- WaitingPermission CC text 已 R2.1.3 HIGH-3 verified: `Waiting for permission…` + dimColor + MessageResponse height=1 (CC AssistantToolUseMessage.tsx:240).
- Bash empty-progress fallback `Running…` 已 R2.1.3 HIGH-2 verified: BashTool/UI.tsx:148.

### Sanitizer 加项 (spec-1.9 specific)

- ToolUse Input map 中可能含**文件路径** (Read) / **命令字符串** (Bash); 路径含 `/Users/<name>/` 必匿名 → `/Users/user/`
- Bash command output 中任何 shell prompt / hostname / username 必匿名
- 截图 crop 仅 ToolUse block region, 不含 surrounding session context (减泄漏面)

### 半天卡退路径 B (spec-1.9 specific)

若任一 ToolUse fixture 7 个内 capture 卡住 (尤其 #2 generic / #5 permission 配置不通):
- 当前卡住 fixture mark `deferred: mock` in metadata, 用 spec-1.9 R2.1.3 placeholder mock; 优先抓其他 fixture
- spec-1.9 R3 errata 期补真 fixture 替 mock
- T-10 FROZEN 前所有 7 fixture 必真 (mock 是 stage 1 卡退, FROZEN gate 不接受 mock)
