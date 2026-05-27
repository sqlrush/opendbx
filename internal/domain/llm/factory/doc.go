// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// Package factory builds an llm.Provider from config (spec-1.20 D-7).
// Separate package (not llm) to avoid the llm ↔ anthropic import cycle:
// it imports llm + the adapter packages + config.
//
// tier auto-degrade (spec-3.11) wraps the factory output in a decorator —
// NOT inside any Provider.Stream (R2 H-6). spec-1.20 builds a single
// concrete provider.
package factory
