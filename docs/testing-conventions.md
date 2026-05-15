# Testing Conventions — opendbx

> spec-0.11-test-framework § D-6 SSOT.
>
> opendbrb 为权威源, opendbx/docs/testing-conventions.md 由 spec-0.11
> T-10 手动 `cp` + `diff -q` 验证镜像一致.

---

## 1. 5 层测试 (CLAUDE.md 规则 8)

每层职责 / 目录 / 工具栈对照:

| Layer | 目录 | 范围 | 工具 | 覆盖率门槛 |
|---|---|---|---|---|
| 1. 单元 (unit) | `*_test.go` 与生产代码同包 | 边界 / 错误路径 / 并发 race / 表驱动 | stdlib `testing` + `tablerun` + `must` | Tier (见下) |
| 2. 集成 (integration) | `tests/integration/` | 跨模块 + testcontainers PG + fake LLM | (spec-1.X 落地) | — |
| 3. E2E | `tests/e2e/` | 真 LLM + 真 PG + PTY UI | `uitest` + (spec-1.X) | — |
| 4. 故障注入 (chaos) | `tests/chaos/` | 网络分区 / OOM / 磁盘满 / LLM 空响应 | (spec-2.X) | nightly only |
| 5. 性能基线 (perf) | `tests/perf/` | 每 stage freeze baseline.json | `go test -bench` + `benchstat` | spec-0.8 D-2 |

### 覆盖率分层 (spec-0.8 D-1 + spec-0.11 T-2.5)

| Tier | 阈值 | 范围 |
|---|---|---|
| Core | ≥ 85% | `internal/platform/{errcode, logger, version}` |
| **Tool** | **≥ 90%** | `tools/*` (6 工具) + `internal/testing/*` (4 包) |
| Other | ≥ 75% | 其他非 exempt 包 |
| Exempt | 无 | `internal/entrypoints` / `cmd/opendbx` / `internal/platform/{config, rpc}` / spec-0.7 era tools |
| Total | ≥ 80% | 全项目聚合 |

---

## 2. 命名与组织

### 2.1 文件 / 函数命名

- 测试文件: `<source>_test.go` 与生产代码同目录、同 package
- 测试函数:
  - `TestXxx(t *testing.T)` — 普通测试
  - `BenchmarkXxx(b *testing.B)` — benchmark
  - `TestXxx_Sub(t *testing.T)` — 不要嵌套深; 用 `t.Run` 而非命名子函数
- helper-process: `TestXxxHelper` + env switch `GO_*_HELPER=<mode>` (参见 uitest pattern)

### 2.2 Table-driven 范式

**禁止手写 `for _, c := range cases { t.Run(c.name, ...) }`**——用 `internal/testing/tablerun`:

```go
import testrun "github.com/sqlrush/opendbx/internal/testing/tablerun"

func TestExample(t *testing.T) {
    cases := []struct {
        Name string // 必需; tablerun 反射读
        In   int
        Want int
    }{
        {Name: "zero", In: 0, Want: 0},
        {Name: "positive", In: 1, Want: 2},
    }
    testrun.Run(t, cases, func(t *testing.T, c struct{...}) {
        if got := double(c.In); got != c.Want {
            t.Errorf("%s: got %d, want %d", c.Name, got, c.Want)
        }
    })
}
```

- **默认 serial** — 适用大多数测试 (包括 `t.Setenv` / `os.Chdir` / global mutation)
- **`testrun.RunParallel`** — 仅当显式审计无 process-global state 时使用
- **`Skippable` interface** — case 类型实现 `SkipReason() string` 非空 → 跳过

### 2.3 `testdata/` 布局

```
package/
  source.go
  source_test.go
  testdata/
    golden/
      TestSomeName.golden               # default-name fixture
      TestOther/sub.golden              # t.Run subtest fixture
    fixtures/                            # input data
      input.json
      malformed.yaml
```

### 2.4 Golden file

**禁止手写 fixture 比对**——用 `internal/testing/golden`:

```go
import gold "github.com/sqlrush/opendbx/internal/testing/golden"

func TestRender(t *testing.T) {
    got := render(input)
    gold.CompareString(t, "", got)  // 默认路径: testdata/golden/TestRender.golden
}
```

**更新 golden**:
```bash
go test -update ./internal/your-pkg     # 包级 — 安全
```

**禁止** `go test -update ./...` — 跨包传递 `-update` flag 可能被未注册它的包拒绝。

### 2.5 Assertion helpers

**禁止重复定义 mustParse / mustCheck / mustGit 等**——用 `internal/testing/must`:

```go
import "github.com/sqlrush/opendbx/internal/testing/must"

data := must.File(t, "fixture.json")
var cfg Config
must.JSON(t, data, &cfg)

err := someOp()
must.NoErr(t, err)
must.ErrCode(t, otherOp(), "PKG.SOMETHING_FAILED")  // errcode.Error.Code() check

path := must.WriteTemp(t, "blob.txt", "content")     // t.TempDir 自动管理
must.WriteFile(t, dir, "name.txt", []byte("..."))    // caller 控制 dir
must.WriteFileAt(t, "full/path/file.txt", []byte)    // mkdir -p + write 一气呵成
```

---

## 3. 内部测试包目录

`internal/testing/` 是测试基础设施专用 (production code 禁止 import):

| 包 | 职责 |
|---|---|
| `internal/testing/tablerun` | Table-driven 范式 (Run / RunParallel / Skippable) |
| `internal/testing/must` | Assertion helpers (File / WriteTemp / WriteFile / WriteFileAt / JSON / NoErr / ErrCode) |
| `internal/testing/golden` | Golden file + `-update` flag (Compare / CompareString / CompareFile + Update) |
| `internal/testing/uitest` | PTY + vt10x cell-grid harness (Term / CellGrid / Send / WaitFor / SnapshotGolden); 平台 Unix-only |

依赖方向: 测试代码可 import 这 4 包之一/全部; 4 包之间允许 `uitest → golden` (snapshot 桥接), 其他无依赖.

---

## 4. Race detector 与覆盖率门槛

### 4.1 Race detector 必跑 (CLAUDE.md 规则 9)

```bash
go test -race -count=1 ./...
```

任何 race 报告**阻 merge**. CI `GORACE=halt_on_error=1` 启用 (spec-0.9).

### 4.2 覆盖率门槛由 `tools/coverage-gate` 强制 (spec-0.8 D-1)

```bash
make gate                # 含 coverage-gate
# 等价于:
go test -race -coverprofile=coverage.out ./...
go run ./tools/coverage-gate
```

不达阈值**阻 merge**.

---

## 5. 演进协议

### 5.1 新 helper 加入 `internal/testing/*`

1. 新 helper 必须在已有 spec 中找到至少 2 个使用场景 (防过度抽象)
2. 走 spec patch 或新 spec (视改动范围)
3. godoc 标 "added in spec-X.Y"

### 5.2 改既有 helper signature

被 100+ test 调用的 helper 改 signature = **BREAKING**:
1. commit msg `feat!:` + body `BREAKING CHANGE:` (spec-0.10 D-4 commit-lint enforce)
2. spec patch 列出影响 test 文件清单 + 各文件 retrofit 计划
3. 不允许 "先 deprecate 后删除" 两阶段—直接改 + retrofit 同 PR

### 5.3 改 PTY 接口

`uitest.Terminal` 改动需要回测 spec-0.11.5 视觉测试是否兼容; spec-0.11.5 起草人需在 R1 评审本 spec D-4 接口稳定性.

### 5.4 更新 golden 流程

1. 跑包级 `go test -update ./your-pkg`
2. `git diff testdata/golden/` 人工审 — 任何视觉差异都要解释
3. commit 含 `update golden` 关键词
4. 不允许跨包批量 update (会触发 `-update` flag 冲突)

---

## 6. 历史

- 2026-05-15: spec-0.11 T-10 初版 (`internal/testing/{tablerun, must, golden, uitest}` 4 包落地 + 5 层测试 + tier 覆盖率门槛 + 演进协议)
