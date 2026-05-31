// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File connerrors.go — CONN.* errcode sentinels for connection resolution
// (spec-1.19 D-8; 规则 7 三件套). These are RUNTIME resolution failures
// returned by ActiveConnection / ResolveDSN; config-time validation failures
// go through CONFIG.VALIDATION_FAILED instead. Hints never contain a
// password/DSN.

package bootstrap

import (
	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

//nolint:gochecknoglobals // spec-0.6 contract: errcode sentinels are package-level.
var (
	// ErrUnknownAlias — the requested connection alias is not configured.
	ErrUnknownAlias = errcode.Register(
		"CONN.UNKNOWN_ALIAS",
		"指定的连接 alias 不存在",
		"检查 --connection-alias / default_connection 与 config connections[].alias 是否一致",
	)
	// ErrNoConnection — no connections are configured at all.
	ErrNoConnection = errcode.Register(
		"CONN.NO_CONNECTION",
		"未配置任何数据库连接",
		"在 config connections 添加一个连接 (alias + driver + dsn 或 host/database/user)",
	)
	// ErrAmbiguous — multiple connections but no active one selected.
	ErrAmbiguous = errcode.Register(
		"CONN.AMBIGUOUS",
		"配置了多个连接但未指定使用哪个",
		"用 --connection-alias <alias> 指定, 或在 config 设 default_connection",
	)
	// ErrUnsupportedFields — fields mode used with a driver that has no
	// DSNComposer capability (only postgres composes fields in this stage).
	ErrUnsupportedFields = errcode.Register(
		"CONN.UNSUPPORTED_FIELDS",
		"该 driver 不支持结构化字段连接配置",
		"该 driver 仅支持 dsn 模式; 用 dsn 字段提供完整连接串 (字段组装当前仅 postgres)",
	)
)
