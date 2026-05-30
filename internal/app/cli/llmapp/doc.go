// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package llmapp is the spec-1.20 D-6 production chat Model, upgraded by
// spec-1.21 D-6 to multi-turn function-calling: it wires spec-1.16
// natural-mode input through a diagnose.Loop (which itself drives an
// llm.Provider) into the spec-1.6 TokenStream + spec-1.9 block.ToolUse /
// spec-1.9b block.ToolResult scrollback, so `opendbx interact` is a real
// multi-turn tool-using LLM chat (replacing the spec-1.17 demoapp).
//
// PURE Update + Cmd (spec-1.15 contract, R2 CRIT-1): Update never starts
// a goroutine / does IO. ActionSubmit allocates ctx/cancel/TokenStream/
// ctrl chan (pure) and returns loopStartCmd; the goroutine + diagnose.
// Loop.Run + provider IO + cancel fire live in Cmds / Cleanup.
//
// text/control split (R2 CRIT-2 + spec-1.21 D-6 union extension):
// renderable text → TokenStream (Drained in View); control
// (VisibleContent / ThinkingToken + *llm.ToolUse / *llm.ToolResult / Finish +
// TermCode) → ctrl chan → reader-Cmd → Update. Thinking / tool events
// never enter the FROZEN streaming.Chunk.
//
// cli-tree root (peer to cmd/opendbx); not part of the §3.1 render DAG.
package llmapp
