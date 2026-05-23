// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File generic.go — generic fallback HeaderRenderer (spec-1.9 D-5).
// Used by ToolUse.Render when adapter Registry.Lookup returns nil
// (tool not registered). Produces "Name(key=val, key2=val2)" compact
// summary truncated to ctx.Cols/2 max; **NOT** equivalent to R1
// generic JSON pretty (R2 HIGH-4 absorb).

package adapter

import (
	"fmt"
	"sort"
	"strings"
)

// Generic implements HeaderRenderer only (no Progress / Queued).
// Compact summary form per CC fallback behavior; not JSON pretty.
type Generic struct{}

// RenderHeader produces "Name(k=v, k2=v2)" truncated to maxCols/2.
// Keys sorted for determinism. Empty input map → "Name()".
//
// Caller (ToolUse.Render) passes the actual tool Name through input
// via a synthetic "_name" key (managed in toolcall.go); Generic looks
// for it; falls back to "(unnamed)" if absent.
func (Generic) RenderHeader(input map[string]any, ctx Context) (string, error) {
	name, _ := input["_name"].(string)
	if name == "" {
		name = "(unnamed)"
	}
	maxLen := ctx.Cols / 2
	if maxLen < 8 {
		maxLen = 8
	}
	args := compactArgs(input, maxLen-len(name)-2) // -2 for parens
	return fmt.Sprintf("%s(%s)", name, args), nil
}

func compactArgs(input map[string]any, budget int) string {
	if budget <= 0 || len(input) == 0 {
		return ""
	}
	keys := make([]string, 0, len(input))
	for k := range input {
		if strings.HasPrefix(k, "_") {
			continue // skip synthetic keys
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return ""
	}
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteString(", ")
		}
		pair := fmt.Sprintf("%s=%v", k, input[k])
		if b.Len()+len(pair) > budget {
			if b.Len() == 0 {
				// Single pair too long: truncate.
				if budget > 3 {
					b.WriteString(pair[:budget-1])
					b.WriteRune('…')
				}
			} else {
				b.WriteString("…")
			}
			break
		}
		b.WriteString(pair)
	}
	return b.String()
}
