// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File parse.go — frontmatter split + YAML decode + Extra collection
// (spec-2.1 D-3). Pure, no I/O.
//
// Boundary detection (review HIGH-3): the closing fence is the first line
// that is EXACTLY "---" at column 0. That mirrors YAML's own document-end
// rule — a "---" inside a block scalar is necessarily indented, so it can
// never be at column 0 and never false-matches. Frontmatter SEMANTICS
// (decode + Extra + depth) then go through yaml.Node, which handles the
// block-scalar content correctly.

package skills

import (
	"bytes"

	"github.com/sqlrush/opendbx/internal/platform/errcode"
	yaml "go.yaml.in/yaml/v3"
)

const (
	// maxSkillSize caps a SKILL.md at 1 MiB. This is an opendbx safety bound
	// (not a CC limit) — DoS guard before any parsing happens.
	maxSkillSize = 1 << 20
	// maxYAMLDepth rejects frontmatter nesting >= 32 (anti-bomb; mirrors the
	// spec-0.4 config value, reimplemented locally so app/skills stays a leaf
	// that does not import platform/config).
	maxYAMLDepth = 32
)

var (
	utf8BOM   = []byte{0xEF, 0xBB, 0xBF}
	fenceLine = []byte("---")
)

// Parse splits a SKILL.md into frontmatter + body and decodes the frontmatter.
// It does NOT validate content (call Validate separately) so callers (2.2)
// can parse a batch and report all failures together. Returns a fresh Skill;
// inputs are never mutated.
func Parse(content []byte, src SkillSource) (Skill, error) {
	if len(content) > maxSkillSize {
		return Skill{}, errcode.Newf(ErrTooLarge.Code(),
			"SKILL.md is %d bytes, exceeds the %d byte limit", len(content), maxSkillSize)
	}

	fm, body, err := splitFrontmatter(content)
	if err != nil {
		// errcode-lint:exempt -- spec-2.1 D-3: splitFrontmatter returns SKILL.* errcode sentinels.
		return Skill{}, err
	}

	var node yaml.Node
	if uerr := yaml.Unmarshal(fm, &node); uerr != nil {
		return Skill{}, errcode.Wrap(ErrParseError.Code(), uerr, "", "")
	}

	root := &node
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		root = node.Content[0]
	}

	if depth := yamlNodeDepth(root); depth >= maxYAMLDepth {
		return Skill{}, errcode.Newf(ErrTooDeep.Code(),
			"frontmatter YAML nesting depth %d >= %d", depth, maxYAMLDepth)
	}

	var schema Schema
	if root.Kind != 0 { // empty frontmatter leaves a zero node; skip decode
		if derr := root.Decode(&schema); derr != nil {
			return Skill{}, errcode.Wrap(ErrParseError.Code(), derr, "", "")
		}
	}
	schema.Extra = collectExtra(root)

	return Skill{Schema: schema, Body: string(body), Source: src}, nil
}

// splitFrontmatter returns the frontmatter bytes (between the fences) and the
// body bytes (after the closing fence's newline). Strips a leading UTF-8 BOM.
// The body is byte-faithful: the closing fence line's trailing newline is
// consumed, everything after it is the body verbatim.
func splitFrontmatter(content []byte) (fm, body []byte, err error) {
	content = bytes.TrimPrefix(content, utf8BOM)

	firstLine, afterFirst := readLine(content, 0)
	if !isFence(firstLine) {
		return nil, nil, errcode.New(ErrNoFrontmatter.Code(), "", "")
	}

	for off := afterFirst; off < len(content); {
		line, next := readLine(content, off)
		if isFence(line) {
			return content[afterFirst:off], content[next:], nil
		}
		off = next
	}
	// Reached EOF without a closing fence. The opening "---" with nothing
	// after it (or only a partial line) is unterminated.
	return nil, nil, errcode.New(ErrUnterminatedFrontmatter.Code(), "", "")
}

// readLine returns the line starting at off (without its newline) and the
// offset of the next line's start. When the line is the last (no trailing
// newline), next == len(b).
func readLine(b []byte, off int) (line []byte, next int) {
	rel := bytes.IndexByte(b[off:], '\n')
	if rel < 0 {
		return b[off:], len(b)
	}
	return b[off : off+rel], off + rel + 1
}

// isFence reports whether line is exactly "---" at column 0 (CRLF tolerated).
// Any leading whitespace makes it != "---", so the column-0 guard is implicit.
func isFence(line []byte) bool {
	return bytes.Equal(bytes.TrimSuffix(line, []byte{'\r'}), fenceLine)
}

// collectExtra gathers every top-level frontmatter key that does NOT map to an
// explicit Schema field into a map (forward-compat; never dropped, review
// HIGH-1). Returns nil when there are no extra keys.
func collectExtra(root *yaml.Node) map[string]any {
	if root == nil || root.Kind != yaml.MappingNode {
		return nil
	}
	known := knownSchemaKeys()
	var extra map[string]any
	for i := 0; i+1 < len(root.Content); i += 2 {
		key := root.Content[i].Value
		if known[key] {
			continue
		}
		var v any
		if err := root.Content[i+1].Decode(&v); err != nil {
			continue // best-effort: a value we cannot decode is simply skipped
		}
		if extra == nil {
			extra = make(map[string]any)
		}
		extra[key] = v
	}
	return extra
}

// yamlNodeDepth returns the maximum container-nesting depth of the node tree.
// Scalars/aliases contribute 0; each mapping/sequence adds 1. A flat
// frontmatter mapping is depth 1.
func yamlNodeDepth(n *yaml.Node) int {
	if n == nil {
		return 0
	}
	switch n.Kind {
	case yaml.DocumentNode:
		max := 0
		for _, c := range n.Content {
			if d := yamlNodeDepth(c); d > max {
				max = d
			}
		}
		return max
	case yaml.MappingNode, yaml.SequenceNode:
		max := 0
		for _, c := range n.Content {
			if d := yamlNodeDepth(c); d > max {
				max = d
			}
		}
		return max + 1
	default:
		return 0
	}
}
