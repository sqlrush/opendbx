// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File errors.go — DB.* errcode sentinels (spec-1.18 D-5; 规则 7 三件套).
//
// Shared cross-driver taxonomy: every driver (postgres now; mysql/oracle/
// opengauss later) maps its own native errors onto THESE codes via its own
// private classify() — mirroring the llm.Provider precedent where llm.Err*
// are shared and each provider (anthropic/openai) does its own mapping.
//
// 原则 3 / security.md: Hints MUST NOT contain DSN / password / host. The
// driver classify() returns a sanitized error whose Error() never renders
// the underlying pgx text (which may embed the DSN); see postgres
// errors_classify.go.

package db

import (
	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

//nolint:gochecknoglobals // spec-0.6 contract: errcode sentinels are package-level.
var (
	// ErrConnectFailed — could not establish a connection (SQLSTATE class
	// 08 / invalid_catalog_name 3D000 / network / DNS / refused). Not a
	// degraded answer — opendbx surfaces the code so the caller fixes
	// connectivity (原则 3 only governs LLM degradation, but DB connect
	// errors must likewise be explicit, never silently swallowed).
	ErrConnectFailed = errcode.Register(
		"DB.CONNECT_FAILED",
		"数据库连接失败",
		"检查 host / port / database 名与网络可达性; 确认实例在线",
	)
	// ErrAuthFailed — authentication / authorization rejected (SQLSTATE
	// class 28 / insufficient_privilege 42501).
	ErrAuthFailed = errcode.Register(
		"DB.AUTH_FAILED",
		"数据库认证或授权失败",
		"检查用户名 / 密码 / 权限与 pg_hba.conf; 确认账号未锁定",
	)
	// ErrTimeout — context deadline exceeded / cancelled / network timeout
	// during an Open / Ping / HealthCheck.
	ErrTimeout = errcode.Register(
		"DB.TIMEOUT",
		"数据库操作超时",
		"提高连接 / 查询超时, 或检查实例负载与网络延迟",
	)
	// ErrQueryFailed — a SQL statement failed for a reason other than the
	// connection (syntax / missing object / other SQLSTATE).
	ErrQueryFailed = errcode.Register(
		"DB.QUERY_FAILED",
		"数据库查询失败",
		"检查 SQL 语句与引用对象是否存在; 查看服务端日志获取 SQLSTATE",
	)
	// ErrUnavailable — server cannot service the request right now
	// (SQLSTATE class 53 insufficient_resources / 57 operator_intervention).
	ErrUnavailable = errcode.Register(
		"DB.UNAVAILABLE",
		"数据库暂不可用 (资源不足或运维介入)",
		"检查磁盘 / 连接数 / 内存; 确认实例未在重启或维护",
	)
	// ErrDriverUnknown — db.Open called with a driver name that was never
	// Register-ed. Runtime (not init) error so it carries a code+Hint.
	ErrDriverUnknown = errcode.Register(
		"DB.DRIVER_UNKNOWN",
		"未注册的数据库 driver",
		"检查 driver 名是否受支持 (当前: postgres); 确认对应 driver 包已 import",
	)
)
