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
  1. 跑 `make ui-block-golden -update-visual` 拉新 fixture
  2. 用户视觉 review 新 golden 是否 acceptable
  3. 若 acceptable: commit; 若不: spec-1.7 errata 改 const 与 CC 对齐
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
