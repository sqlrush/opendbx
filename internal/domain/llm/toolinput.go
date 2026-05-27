// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush
//
// File toolinput.go — shared tool-input JSON guard.
//
// spec-1.20.1 D-7 / Q9: moved (errata-move, behavior unchanged) from the
// anthropic adapter to the domain llm package so every provider adapter
// (anthropic + openai) reuses ONE guard. spec-1.20 errata, no patch tag.

package llm

import (
	"bytes"
	"encoding/json"
	"io"
)

// Tool-input JSON guard bounds (spec-1.20 D-4 / R2 MED-1 / T-10a HIGH-2;
// shared by all provider adapters since spec-1.20.1).
const (
	MaxToolInputBytes = 256 * 1024 // 256 KB per tool input
	MaxToolInputDepth = 32
	// MaxToolBlocks caps concurrent tool blocks per turn (spec-1.20 T-10a
	// HIGH-2): an unbounded accumulation map lets a hostile/confused model
	// allocate N × MaxToolInputBytes of buffers.
	MaxToolBlocks = 64
)

// DecodeToolInput decodes a tool-call input JSON with object/size/depth/EOF
// guard. Empty input → empty object. Returns ErrDecodeFailed on oversize,
// malformed, trailing-data, non-object, or too-deep input (defends against
// hostile/confused-model tool arguments — 痛点 defense).
func DecodeToolInput(raw []byte) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil // empty input → empty object
	}
	if len(raw) > MaxToolInputBytes {
		return nil, ErrDecodeFailed
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, ErrDecodeFailed
	}
	// Reject trailing data after the first JSON value — a single Decode
	// accepts "{}<garbage>". The input must be exactly one object.
	if _, err := dec.Token(); err != io.EOF {
		return nil, ErrDecodeFailed
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return nil, ErrDecodeFailed // tool input must be a JSON object
	}
	if jsonDepth(v, 1) > MaxToolInputDepth {
		return nil, ErrDecodeFailed
	}
	return obj, nil
}

// jsonDepth returns the maximum nesting depth of a decoded JSON value.
func jsonDepth(v any, cur int) int {
	max := cur
	switch t := v.(type) {
	case map[string]any:
		for _, child := range t {
			if d := jsonDepth(child, cur+1); d > max {
				max = d
			}
		}
	case []any:
		for _, child := range t {
			if d := jsonDepth(child, cur+1); d > max {
				max = d
			}
		}
	}
	return max
}
