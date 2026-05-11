# Codex Review：spec-0.5 logger wiring 修复结论

日期：2026-05-11  
仓库：`sqlrush/opendbx`  
PR：https://github.com/sqlrush/opendbx/pull/8  
Head commit：`47ed1696f94144765f34ef4064e5ecf04473d2c1`

## Review 范围

本次 review 覆盖 spec-0.5 logger 实施完成后的真实接线层，重点检查 logger 是否真正接入 CLI/config 执行路径：

- `cmd/opendbx` root command wiring
- `internal/entrypoints` logger relay
- `internal/platform/config` output logger 字段
- `internal/platform/logger` debug mode / filter / session 初始化
- logger 测试的 sidecar 隔离与回归覆盖

## Findings

### HIGH-1：`OutputConfig` 缺失 spec-0.5 要求的 logger 配置字段

`internal/platform/config.OutputConfig` 没有暴露 `log_level` / `log_path`，但 spec-0.5 D-8 要求 logger 初始化支持 config 驱动。

影响：

- `output.log_level` 无法覆盖 logger min level。
- `output.log_path` 无法覆盖 logger main text log path。
- `admin config dump-schema` / `dump-env-map` 没有 logger config 契约。

修复：

- 新增 `Output.LogLevel` / `Output.LogPath`，带 YAML / JSON / ENV / validation tags。
- 默认值新增 `log_level=debug`、`log_path=""`。
- relay 只在 `Output.LogLevel` 来源不是 default 时传入 logger，保留 `OPENDBX_DEBUG_LOG_LEVEL` fallback 行为。

### HIGH-2：cobra 已解析的 logger flags 没有完整传入 logger init

`cmd/opendbx/root.go` 之前只传了 `DebugToStderr`，没有传：

- `--debug`
- `--debug=<pattern>`
- `--debug-file`
- `--session-id`
- config-resolved `Output.LogLevel`
- config-resolved `Output.LogPath`

影响：

- 真实 cobra 执行时 logger 行为依赖 `os.Args` fallback，而不是已解析的 flag state。
- `cmd.SetArgs()` 的 in-process 测试无法覆盖这类 bug，因为 `os.Args` 仍然指向测试二进制。
- `--session-id` 不能稳定 logger 文件名 / sidecar session id。

修复：

- 在 `internal/entrypoints` 新增 `InitLoggerFromConfigAndCLI(cfg, inputs)`。
- root 现在显式传入 cobra 解析出的 `Debug`、`DebugFile`、`DebugToStderr`、`SessionID`。
- logger `InitInput` 新增 `DebugEnabled` / `DebugFilter`。
- 保留 argv fallback，继续满足 CC-first 的 argv/env 行为兼容。

### MED-1：logger sidecar 测试不 hermetic

多处 logger 测试设置了 temp `--debug-file`，但没有设置 `HOME`。结果 main text log 写到 temp path，JSONL sidecar 仍会尝试写真实 `~/.opendbx/debug`。

影响：

- sandbox / local CI 可能因真实 HOME 权限失败。
- 测试可能污染开发者真实 `~/.opendbx/debug`。

修复：

- 给相关 logger 测试补 `t.Setenv("HOME", tmp)`。
- sidecar 输出现在被限制在每个测试自己的临时 HOME 下。

### MED-2：缺少 relay / session / parsed debug wiring 回归测试

原测试大多直接调用 `logger.Init()`，没有覆盖 entrypoints relay 把 CLI/config 转成 `logger.InitInput` 的路径。

影响：

- `--session-id` 和 parsed debug flag wiring bug 可能再次回归且不被测试发现。

修复：

- 扩展 `internal/entrypoints/logger_relay_test.go`：
  - 验证 parsed debug settings 能启用 logger；
  - 验证 custom debug file 收到 CC text output；
  - 验证 sidecar 使用指定 `SessionID`。
- 增加 logger package tests：
  - `InitInput.DebugEnabled` 不依赖 argv `--debug` 也能启用写入；
  - `InitInput.DebugFilter` 不依赖 argv `--debug=<pattern>` 也能过滤。

## 最终结论

本次 review 发现的问题已全部在 PR #8 修复。

修复后 spec-0.5 logger wiring 满足预期：

- config 层暴露 logger 字段；
- cobra parsed flags 驱动 logger 行为；
- `--session-id` 能稳定 logger session 输出；
- CC-style argv/env fallback 仍保留；
- JSONL sidecar 仍独立于 `--debug-file`；
- 测试不会写真实用户 debug 目录。

## 验证结果

本地验证：

```text
go test ./internal/platform/config ./internal/platform/logger ./internal/entrypoints ./cmd/opendbx
go test ./...
make gate
go test -race -coverprofile=/private/tmp/opendbx-coverage.out ./...
```

GitHub Actions run `25648849515`：

```text
Validate (lint / fmt / vet)                 PASS
Build (matrix) (ubuntu-latest)              PASS
Build (matrix) (macos-latest)               PASS
Unit Test (race + cover)                    PASS
Import Rules Check (spec-0.2 D-5)           PASS
Dep Allowlist Check (spec-0.2 D-6)          PASS
CLI Text Golden (spec-0.2 D-3)              PASS
```

Review close 时 PR 状态：`MERGEABLE CLEAN`。

