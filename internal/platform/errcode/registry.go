// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package errcode

import (
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Definition is a registry record populated by Register at package init
// time (via file-scope `var Err = Register(...)`).
type Definition struct {
	// Code is the canonical identifier, e.g. "LOGGER.WRITER_CLOSED".
	Code string
	// Message is the default single-sentence description used when
	// New(code, "", "") is called.
	Message string
	// Hint is the default remediation suggestion.
	Hint string
	// Module is the prefix before the first dot — populated automatically
	// from Code at Register time. Used by docs_gen to group entries.
	Module string
}

// Sentinel is the typed value returned by Register. Backed by
// *structuredErr at runtime so errors.Is symmetry works in both directions
// (spec § 2.2 + § 1.3 pseudocode).
type Sentinel interface {
	Error
}

// codeRE validates Code against the naming convention (spec § 1.3, R2
// tightened). The grammar:
//
//   - module prefix: starts with uppercase letter, 2+ chars of [A-Z0-9],
//     NO underscore in the prefix.
//   - one or more dotted segments: each starts with uppercase letter,
//     2+ chars of [A-Z0-9_].
//
// Examples accepted: LOGGER.WRITER_CLOSED / CONFIG.INVALID_PATH /
// LLM.STREAM.EMPTY_CONTENT (multi-dot allowed).
// Examples rejected: LLM_CLIENT.X (underscore in module) / lower.case /
// X.Y (segments too short).
var codeRE = regexp.MustCompile(`^[A-Z][A-Z0-9]+(?:\.[A-Z][A-Z0-9_]+)+$`)

// testPrefix marks codes that are excluded from All() / docs_gen output.
// Tests that need to Register codes should use this prefix.
const testPrefix = "TEST."

// registry holds all definitions keyed by Code. Protected by an RWMutex so
// the read-heavy hot path (Lookup called on every New/Newf/Wrap call;
// All called by docs_gen + manifest test) does not serialise behind
// the write-only Register/unregister paths. go-reviewer H-1 R2 alignment.
var (
	mu       sync.RWMutex
	registry = map[string]Definition{}
)

// Register declares a code. See spec § 2.2 for the full contract:
//
//   - Conflicting registration (same code, different msg or hint) → panic.
//   - Identical re-registration (all three fields equal) → no-op, returns
//     the existing Sentinel.
//   - Returns a *structuredErr typed as Sentinel interface so errors.Is
//     symmetry holds via Code matching.
//
// Caller convention: file-scope `var Err = Register(...)` (spec § 2.2.1).
func Register(code, msg, hint string) Sentinel {
	if !codeRE.MatchString(code) {
		panic("errcode: Register code violates naming convention (MODULE.NOUN_VERB): " + code)
	}
	module := moduleFromCode(code)

	mu.Lock()
	defer mu.Unlock()

	if existing, ok := registry[code]; ok {
		if existing.Message == msg && existing.Hint == hint {
			// Identical re-registration: idempotent no-op. Re-construct the
			// sentinel — it's a small heap allocation but keeps the returned
			// value semantically stable for `var Err = Register(...)` at file
			// scope across test re-imports.
			return &structuredErr{code: code, message: msg, hint: hint}
		}
		panic("errcode: Register conflict for " + code +
			" — existing message=" + existing.Message +
			" hint=" + existing.Hint +
			"; new message=" + msg + " hint=" + hint)
	}

	registry[code] = Definition{
		Code:    code,
		Message: msg,
		Hint:    hint,
		Module:  module,
	}
	return &structuredErr{code: code, message: msg, hint: hint}
}

// Lookup returns the Definition for a code, or (zero, false) if unknown.
// Used internally by New/Newf/Wrap; exported for diagnostic tooling.
func Lookup(code string) (Definition, bool) {
	mu.RLock()
	defer mu.RUnlock()
	def, ok := registry[code]
	return def, ok
}

// All returns every registered Definition sorted by Code, EXCLUDING the
// TEST.* prefix (reserved for test fixtures; spec § 2.2 + R-9 mitigation).
// Used by the docs_gen tool to produce docs/error-codes.md.
func All() []Definition {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Definition, 0, len(registry))
	for _, def := range registry {
		if strings.HasPrefix(def.Code, testPrefix) {
			continue
		}
		out = append(out, def)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

// unregisterForTesting removes a code from the registry. Test-only helper
// — not exported because production code should never deregister. Tests
// pair Register / unregister inside t.Cleanup to keep the global registry
// clean across subtests.
//
// nolint:unused // consumed by *_test.go in this package and downstream tests
func unregisterForTesting(code string) {
	mu.Lock()
	defer mu.Unlock()
	delete(registry, code)
}

// moduleFromCode extracts the prefix segment before the first dot. Called
// only from Register where codeRE has already passed, so we know there's
// at least one dot.
func moduleFromCode(code string) string {
	idx := strings.IndexByte(code, '.')
	if idx <= 0 {
		return code
	}
	return code[:idx]
}
