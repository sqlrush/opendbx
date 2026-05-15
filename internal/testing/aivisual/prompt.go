// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package aivisual

import (
	_ "embed"
)

// promptTemplate is the frozen evaluator prompt baked into the binary
// at build time. SHA-256 of the embedded bytes is verified by
// TestPromptFrozen — any unintentional edit fails the test.
//
// spec § D-3 + R4 codex round-3 MED-5: prompt changes走 BREAKING
// per § 11.3 (与 R-9 一致).
//
//go:embed testdata/prompt.txt
var promptTemplate string

// Prompt returns the evaluator prompt template.
func Prompt() string {
	return promptTemplate
}
