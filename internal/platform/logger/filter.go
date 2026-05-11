// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package logger

import (
	"regexp"
	"strings"
)

// debugFilter holds a parsed `--debug=<pattern>` filter configuration.
//
// Filter parsing semantics (CC debugFilter.ts:7-57, verbatim):
//
//   - Pattern is a comma-separated list of category names.
//   - Names prefixed with "!" mean "exclude this category".
//   - Mixing inclusive and exclusive in the same pattern is treated as
//     "no filter" (fail-soft; CC same behaviour).
//   - All category names are normalised to lowercase for case-insensitive
//     matching.
//
// Example patterns (CC reference):
//
//	"api,hooks"   → include = {api, hooks}; exclusive = false
//	"!1p,!file"   → exclude = {1p, file};   exclusive = true
//	""            → no filter (nil)
//	"a,!b"        → no filter (mixed)
type debugFilter struct {
	include   []string
	exclude   []string
	exclusive bool // true if all entries are "!"-prefixed (exclude mode)
}

// parseDebugFilter parses a `--debug=<pattern>` argument into a debugFilter.
// Returns nil for empty input, no-op patterns, or mixed inclusive/exclusive.
//
// CC parity (debugFilter.ts:17-57):
//   - empty / whitespace-only string → nil
//   - all entries after split are blank → nil
//   - mixed inclusive (no !) and exclusive (with !) → nil (fail-soft)
func parseDebugFilter(filterString string) *debugFilter {
	if strings.TrimSpace(filterString) == "" {
		return nil
	}
	rawParts := strings.Split(filterString, ",")
	parts := make([]string, 0, len(rawParts))
	for _, p := range rawParts {
		p = strings.TrimSpace(p)
		if p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return nil
	}
	hasExclusive := false
	hasInclusive := false
	for _, p := range parts {
		if strings.HasPrefix(p, "!") {
			hasExclusive = true
		} else {
			hasInclusive = true
		}
	}
	if hasExclusive && hasInclusive {
		// Mixed: CC treats this as no filter (silently). We preserve that.
		return nil
	}
	clean := make([]string, 0, len(parts))
	for _, p := range parts {
		clean = append(clean, strings.ToLower(strings.TrimPrefix(p, "!")))
	}
	f := &debugFilter{exclusive: hasExclusive}
	if hasExclusive {
		f.exclude = clean
	} else {
		f.include = clean
	}
	return f
}

// CC debugFilter.ts category extraction regexes (debug.ts:65-104).
//
// IMPORTANT: pattern 3 (MCP server) is checked BEFORE pattern 1 to avoid the
// "MCP server" prefix being mis-classified as `mcp server"name":` category 1.
var (
	mcpServerRE   = regexp.MustCompile(`^MCP server ["']([^"']+)["']`)
	prefixColonRE = regexp.MustCompile(`^([^:\[]+):`)
	bracketRE     = regexp.MustCompile(`^\[([^\]]+)\]`)
	secondaryRE   = regexp.MustCompile(`:\s*([^:]+?)(?:\s+(?:type|mode|status|event))?:`)
)

// extractDebugCategories returns the set of category names embedded in a
// debug message. Lowercase + de-duplicated.
//
// CC parity (debugFilter.ts:65-104):
//   - Pattern 3: `MCP server "<name>": msg`     → ["mcp", "<name>"]
//   - Pattern 1: `category: msg` (non-MCP)      → ["category"]
//   - Pattern 2: `[CATEGORY] msg`                → ["category"]
//   - Pattern 4: msg contains `1p event:` lit   → push "1p"
//   - Pattern 5: secondary `:foo:` mid-string   → push "foo" (if reasonable)
//
// "Reasonable" for pattern 5 means: length < 30 AND no embedded spaces.
func extractDebugCategories(message string) []string {
	var categories []string
	seen := map[string]bool{}
	push := func(c string) {
		c = strings.TrimSpace(strings.ToLower(c))
		if c == "" || seen[c] {
			return
		}
		seen[c] = true
		categories = append(categories, c)
	}

	// Pattern 3 first (MCP server "name") to avoid pattern-1 mis-classification.
	if m := mcpServerRE.FindStringSubmatch(message); m != nil {
		push("mcp")
		push(m[1])
	} else if m := prefixColonRE.FindStringSubmatch(message); m != nil {
		// Pattern 1: prefix before first ":" (rejected if "[" appears before).
		push(m[1])
	}

	// Pattern 2: [CATEGORY] at start. Independent of patterns 1/3.
	if m := bracketRE.FindStringSubmatch(message); m != nil {
		push(m[1])
	}

	// Pattern 4: "1p event:" literal anywhere → category "1p".
	if strings.Contains(strings.ToLower(message), "1p event:") {
		push("1p")
	}

	// Pattern 5: secondary `:something:` mid-string.
	if m := secondaryRE.FindStringSubmatch(message); m != nil {
		secondary := strings.TrimSpace(strings.ToLower(m[1]))
		if len(secondary) < 30 && !strings.Contains(secondary, " ") {
			push(secondary)
		}
	}

	return categories
}

// shouldShowDebugCategories applies a debugFilter to a set of categories.
//
// CC parity (debugFilter.ts:115-138):
//   - filter == nil → true (show everything)
//   - categories empty → false (uncategorised messages excluded by default
//     for security; CC same)
//   - exclusive → true iff NO category is in the exclude list
//   - inclusive → true iff ANY category is in the include list
func shouldShowDebugCategories(categories []string, filter *debugFilter) bool {
	if filter == nil {
		return true
	}
	if len(categories) == 0 {
		return false
	}
	if filter.exclusive {
		for _, c := range categories {
			if containsString(filter.exclude, c) {
				return false
			}
		}
		return true
	}
	for _, c := range categories {
		if containsString(filter.include, c) {
			return true
		}
	}
	return false
}

// shouldShowDebugMessage combines category extraction with filter matching.
// Fast path: nil filter short-circuits to true without extraction (CC parity).
func shouldShowDebugMessage(message string, filter *debugFilter) bool {
	if filter == nil {
		return true
	}
	return shouldShowDebugCategories(extractDebugCategories(message), filter)
}

// containsString is a small helper for slice membership. Used in place of
// slices.Contains so the package keeps its single stdlib boundary (encoding/
// json + os + sync + time + context + runtime).
func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
