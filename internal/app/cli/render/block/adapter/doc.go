// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package adapter provides per-tool render adapters for block.ToolUse
// (spec-1.9 D-2). Each tool (Bash, Read, MCP-tools, etc.) implements
// HeaderRenderer (required) and may optionally implement
// ProgressRenderer and/or QueuedRenderer via Go interface segregation
// (per spec-1.9 R2.1.3 HIGH-1, mirroring CC Tool.ts:605-667 ToolUse
// subset; ToolResult-side render methods are owned by spec-1.9b).
//
// DAG position: block/adapter (idx 7) — leaf within block sub-DAG,
// imports style + width only. block (idx 8) imports block/adapter.
//
// Default singleton Registry is populated via init() in each adapter
// file (e.g., bash.go / read.go). ToolUse.Render performs Lookup by
// tool Name and falls back to Generic when unregistered.
//
// Design: spec-1.9-toolcall-block.md D-2 (R2 HIGH-4 + R2.1 HIGH-2 +
// R2.1.3 HIGH-1).
package adapter
