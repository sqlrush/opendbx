# UI Review Pipeline — opendbx

> spec-0.11.5 D-6 SSOT. opendbrb 权威源，opendbx 镜像通过 T-8 手动 `cp` + `diff -q` 验证 bit-identical。
>
> CLAUDE.md § 3.9 5 层 pipeline 完整操作手册。

---

## 1. 5 层总览

| Layer | 作用 | 工具 | 触发 | 阻 merge |
|---|---|---|---|---|
| 1 静态不变量 | runewidth 行宽 / state-based SGR validator | `internal/testing/uiinvariant` | 每 PR (unit-test job) | 是 |
| 2 PTY + cell golden | vt10x cell-grid 字符级 diff | `internal/testing/uitest` (spec-0.11 D-4) | 每 PR (unit-test job) | 是 |
| 3 freeze pixel diff | ANSI→PNG 像素级 diff | `internal/testing/visualgolden` + freeze + rsvg | 每 PR (ui-visual job) | 是 |
| 4 AI 视觉评审 | Qwen2.5-VL-72B 评审 | `internal/testing/aivisual` + llama-server | label-triggered + secret | 否 (PR comment-only) |
| 5 真终端截图 SOP | iTerm2 / Alacritty / kitty 三终端 × 3 size | PR template + workflow | label-triggered | 是 |

5 层是渐进的：cell 级（毫秒）→ 像素级（秒-分钟）→ AI 评审（分钟）→ 人工真终端。每层抓 / 不抓的问题：

- **Layer 1** 抓：行宽溢出 / ANSI 序列未闭合 / SGR 残留 active 属性。**不抓**：视觉布局、颜色协调。
- **Layer 2** 抓：cell 错位、CJK column 漂移、表格断行。**不抓**：字体差异、边框断裂。
- **Layer 3** 抓：视觉对齐错位、边框断裂、缩进不齐、颜色漂移（>1% 像素差）。**不抓**：< 1% 像素抖动（字体 hinting 噪音）。
- **Layer 4** 抓："看起来不漂亮但都合法" 的细微问题（列宽参差、贴图错误、中英宽度不协调）。**不抓**：明确的代码 bug（Layer 1-3 已覆盖）。
- **Layer 5** 抓：终端兼容性、字体差异、主题切换。

---

## 2. Layer 1 — 静态不变量

```go
import "github.com/sqlrush/opendbx/internal/testing/uiinvariant"

func TestRender(t *testing.T) {
    grid := []string{"中文 hello", strings.Repeat("a", 80)}
    uiinvariant.CheckRowWidth(t, grid, 80)
    uiinvariant.CheckANSI(t, []byte("\x1b[31mred\x1b[0m"))
}
```

`CheckRowWidth` 使用 `runewidth.StringWidth` (EastAsianWidth=false 不变量)。`CheckANSI` 是 state-based SGR validator：支持 0m 通用 reset / 22m 等 targeted reset / 30-37 / 40-47 / 90-97 / 100-107 / 38;5;N + 48;5;N 256-color / 38;2;R;G;B + 48;2;R;G;B 24-bit color。

失败模式：行宽 > cols / 残留 active SGR / 截断 CSI / 未知子参数。

---

## 3. Layer 2 — PTY + cell golden

复用 spec-0.11 D-4 `internal/testing/uitest`。详见 `docs/testing-conventions.md` § 2.4 + § 3。本 spec 不重复。

新方法（spec-0.11.5 T-4 BREAKING patch）:

```go
raw, err := term.ANSIRaw()  // 10 MiB cap; ErrAnsiBufFull on overflow
```

供 Layer 3 visualgolden.Render 输入。

---

## 4. Layer 3 — freeze + pixelmatch pixel diff

```go
import "github.com/sqlrush/opendbx/internal/testing/visualgolden"

func TestRenderVisual(t *testing.T) {
    term := uitest.Term(t, cmd, 80, 24)
    term.WaitFor(t, predicate, time.Second)
    raw, err := term.ANSIRaw()
    if err != nil { t.Fatal(err) }
    png := visualgolden.Render(t, raw, visualgolden.DefaultTheme())
    visualgolden.Compare(t, "snapshot", png, 0.01) // 1% maxMismatchFraction
}
```

**更新 golden**：`go test -update-visual ./internal/your-pkg`

**安装 freeze（dev local）**:
```bash
GOTOOLCHAIN=auto go install github.com/charmbracelet/freeze@v0.2.2
brew install librsvg  # macOS; apt install librsvg2-bin on Linux
```

**CI**：`ui-visual` job 自动安装 freeze + librsvg2-bin + 字体（详 § 5）。`VISUALGOLDEN_REQUIRED=1` 让 Render 在 freeze 缺失时 Fatal 而非 Skip。

**确定性**：CI 用 Linux ubuntu-latest + JetBrains Mono + Noto CJK 字体作为唯一 blessed 渲染环境。dev 机器与 CI 字体不同 → `-update-visual` 必须在 CI 跑一次然后 commit。

**pixelSensitivity vs maxMismatchFraction**:
- `Diff(a, b, pixelSensitivity)`: smaller = stricter; per-pixel YIQ-distance threshold (0..1)
- `Compare(t, name, got, maxMismatchFraction)`: max fraction of differing pixels allowed (0..1, typically 0.01 = 1%)

两者名字故意不同避免反义混淆（spec-0.11.5 R2 claude HIGH-2）。

---

## 5. Layer 4 — Qwen2.5-VL-72B AI 视觉评审

```go
import "github.com/sqlrush/opendbx/internal/testing/aivisual"

func TestRenderAIReview(t *testing.T) {
    if os.Getenv("LOCAL_VL_ENDPOINT") == "" {
        t.Skip("LOCAL_VL_ENDPOINT unset; Layer 4 dev-local opt-in")
    }
    png := visualgolden.Render(t, raw, theme)
    r := &aivisual.Reviewer{Endpoint: os.Getenv("LOCAL_VL_ENDPOINT")}
    report, err := r.Review(t.Context(), png, "table alignment + CJK width")
    if errors.Is(err, aivisual.ErrEndpointDown) {
        t.Skip("AI endpoint unreachable; Layer 4 non-blocking")
    }
    must.NoErr(t, err)
    t.Logf("AI verdict: %s, %d issues", report.Verdict, len(report.Issues))
}
```

**本地部署 SOP（dev 机器，M5 Max 128GB）**:
```bash
# 一次性下载模型
mkdir -p ~/model/Qwen2.5-VL-72B
# (下载 Qwen2.5-VL-72B-Q4_K_M.gguf + mmproj-Qwen2.5-VL-72B-f16.gguf)

# 启动 server
~/llama.cpp/bin/llama-server \
  --model ~/model/Qwen2.5-VL-72B/Qwen2.5-VL-72B-Q4_K_M.gguf \
  --mmproj ~/model/Qwen2.5-VL-72B/mmproj-Qwen2.5-VL-72B-f16.gguf \
  --port 8082 \
  -ngl 99 \
  --ctx-size 8192

# 验证
curl -s http://127.0.0.1:8082/v1/models | jq

# 运行 Layer 4 测试
LOCAL_VL_ENDPOINT=http://127.0.0.1:8082/v1 \
  go test -run TestRenderAIReview ./your/pkg
```

**资源占用**: ~40GB RAM (Q4_K_M)、~3GB mmproj、推理 5-15 tok/s。单次 review ~500 tok ≈ 30-60s。

**评审 prompt frozen**: `internal/testing/aivisual/testdata/prompt.txt` SHA-256 守门（`TestPromptFrozen`）。修改 prompt 走 BREAKING（§ 11.3）。

**6 评审维度**: alignment / border / color / cjk-width / indent / ansi。

---

## 6. Layer 5 — 真终端截图 SOP

适用：label `area:render` / `area:ui` 的 PR（renders demoable change）。

**3 终端 × 3 size = 9 截图必含**:
- iTerm2 120w / 80w / CJK
- Alacritty 120w / 80w / CJK
- kitty 120w / 80w / CJK

**字体配置**:
- iTerm2: Preferences → Profiles → Text → Font: JetBrains Mono 14pt
- Alacritty: `~/.config/alacritty/alacritty.toml` → `font.normal.family = "JetBrains Mono"`
- kitty: `~/.config/kitty/kitty.conf` → `font_family JetBrains Mono`

**强制 enforce**:
- PR template (`.github/PULL_REQUEST_TEMPLATE.md`) 含 9 fixed-field block
- `.github/workflows/pr-screenshots-check.yml` 在 `area:render`/`area:ui` PR 上跑
- 字段必含真实 URL（拒 `<url>` 占位、`example.com` 占位、重复 URL）
- 触发: opened / edited / reopened / labeled / synchronize / unlabeled

---

## 7. PR 工作流

| 阶段 | 谁负责 | 工具 |
|---|---|---|
| 写代码 + Layer 1+2 单测 | 开发者 | go test ./internal/testing/{uiinvariant,uitest}/... |
| Layer 3 golden 更新（如适用）| 开发者 | go test -update-visual ./your/pkg |
| 加 `area:render` / `area:ui` label | 开发者 | gh pr edit --add-label |
| Layer 3 自动跑 | CI | ui-visual job 每 PR |
| Layer 4 自动跑（如 secret 设置）| CI | ci-ui-review job label-triggered |
| Layer 5 截图填 PR description | 开发者 | iTerm2/Alacritty/kitty 截图 |
| Layer 5 strict 验证 | CI | pr-screenshots-check workflow |
| 人工 review | reviewer | PR comment + 5 终端 visual sanity |

---

## 8. 演进协议

### 8.1 加 Layer
完整 5 层已定型；新 Layer 走新 sub-spec。

### 8.2 改 freeze theme 默认值
影响所有 visualgolden 输出 → 走 BREAKING 流程。commit msg `feat!:` + body `BREAKING CHANGE: theme change forces all visual goldens to regenerate`。

### 8.3 改 aivisual prompt.txt
影响 Layer 4 评审一致性 → 走 BREAKING 流程（§ 11.3 与 R-9 一致）。frozen SHA-256 守门。

### 8.4 改 PR template 9 fixed-field
反映工作流变化（如新增 zellij 终端 → 12 字段）。走 spec patch + workflow yaml 同步更新。

### 8.5 改 import-rules-check runewidth 例外
spec-1.14 落地 `internal/app/cli/render/width.Width()` 后，移除 `internal/testing/uiinvariant` 例外 + uiinvariant 切换走 render/width 包。

---

## 9. 历史

- 2026-05-15: spec-0.11.5 T-8 初版（CLAUDE.md § 3.9 落地完整 5 层 pipeline；Layer 1+2+3 每 PR 跑 + Layer 4 label/secret optional + Layer 5 PR template strict 9-field）
