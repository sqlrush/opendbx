// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package llmapp is the spec-1.20 D-6 production chat Model: it wires
// spec-1.16 natural-mode input through an llm.Provider streaming
// completion into the spec-1.6 TokenStream + scrollback, so
// `opendbx interact` is a real single-turn LLM chat (replacing the
// spec-1.17 demoapp demonstrator).
//
// PURE Update + Cmd (spec-1.15 contract, R2 CRIT-1): Update never starts
// a goroutine / does IO. ActionSubmit allocates ctx/cancel/TokenStream/
// ctrl chan (pure) and returns streamStartCmd; the goroutine + provider
// IO + cancel fire live in Cmds / Cleanup.
//
// text/control split (R2 CRIT-2): renderable text → TokenStream (Drained
// in View); control (VisibleContent / Thinking / ToolUses / FinishReason)
// → ctrl chan → reader-Cmd → Update. Thinking/ToolUses never enter the
// FROZEN streaming.Chunk.
//
// cli-tree root (peer to cmd/opendbx); not part of the §3.1 render DAG.
package llmapp
