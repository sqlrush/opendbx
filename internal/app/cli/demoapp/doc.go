// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package demoapp is the spec-1.17 D-6b minimal demonstrator Model.
// It wires the spec-1.15/1.16/1.17 input + cursor + history pipeline
// behind `opendbx interact` so the local dev loop can exercise the
// production entry path end-to-end.
//
// Replacement path: spec-1.20 (LLM client) introduces a real
// production Model carrying provider state, scrollback, status segments,
// etc. demoapp.New is the placeholder in bootstrap.LaunchInteractiveTUI;
// spec-1.20 swaps it out.
//
// DAG: demoapp is a cli-tree root caller (peer to cmd/opendbx). It is
// NOT part of the § 3.1 render DAG; layers.go AppLayer dependency
// direction (no internal/* package imports demoapp) is the enforcement.
//
// Imports: program (10) + keybindings (9.6) + input (9.5) + scheduler /
// buffer / render/terminal — all forward; no demoapp consumer inside
// internal/* exists or will exist.
package demoapp
