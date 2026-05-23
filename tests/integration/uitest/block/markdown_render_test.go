// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

//go:build !windows

package block_test

import (
	"os"
	"strings"
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/block"
	"github.com/sqlrush/opendbx/internal/testing/uiinvariant"
	"github.com/sqlrush/opendbx/internal/testing/visualgolden"
)

// TestMarkdownVisualGolden consumes 5 Markdown CC fixtures parked
// under tests/integration/uitest/block/testdata/visual/Markdown*/.
//
// Fixture coverage (spec-1.11 D-8 § 4.2 INT-4):
//
//	MarkdownHeadings   — H1-H6 sequence
//	MarkdownLists      — ordered + unordered + nested 3-level
//	MarkdownFenceCode  — fence with lang label (spec-1.7 delegate)
//	MarkdownTable      — 3x3 basic table
//	MarkdownMixed      — complex doc (heading + paragraph + list + fence + blockquote + table)
//
// MARKDOWN_VISUAL_REQUIRED=1 turns missing fixtures into fatal (CI
// strict); without it tests t.Skip (developer-local convenience).
// Independent env gate per spec-1.11 D-8 lesson (decoupled from
// BLOCK / TOOLUSE / TOOLRESULT / COMPACT). 5th independent block env gate.
func TestMarkdownVisualGolden(t *testing.T) {
	cases := []struct {
		name   string
		source string
		cols   int
	}{
		{
			name: "MarkdownHeadings",
			source: "# H1 Heading\n" +
				"## H2 Heading\n" +
				"### H3 Heading\n" +
				"#### H4 Heading\n" +
				"##### H5 Heading\n" +
				"###### H6 Heading\n",
			cols: 80,
		},
		{
			name: "MarkdownLists",
			source: "Ordered:\n" +
				"1. first\n" +
				"2. second\n" +
				"\nUnordered nested:\n" +
				"- a\n" +
				"  - a1\n" +
				"    - a1.1\n" +
				"- b\n",
			cols: 80,
		},
		{
			name: "MarkdownFenceCode",
			source: "```go\n" +
				"func main() {\n" +
				"    fmt.Println(\"hello\")\n" +
				"}\n" +
				"```",
			cols: 80,
		},
		{
			name: "MarkdownTable",
			source: "| col1 | col2 | col3 |\n" +
				"|------|------|------|\n" +
				"| a    | b    | c    |\n" +
				"| 1    | 2    | 3    |\n" +
				"| x    | y    | z    |\n",
			cols: 80,
		},
		{
			name: "MarkdownMixed",
			source: "# Mixed Document\n\n" +
				"First paragraph with **bold** and *italic* and `code`.\n\n" +
				"> blockquoted line one\n\n" +
				"- list item alpha\n" +
				"- list item beta\n\n" +
				"```python\n" +
				"def hello():\n" +
				"    print('world')\n" +
				"```\n\n" +
				"---\n\n" +
				"| name | value |\n" +
				"|------|-------|\n" +
				"| a    | 1     |\n",
			cols: 80,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx := ctxDefault(tc.cols)
			m := block.NewMarkdown(tc.source)
			buf, err := m.Render(ctx)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			raw := bufferANSI(t, buf)
			uiinvariant.CheckANSI(t, raw)
			png := visualgolden.Render(t, raw, visualgolden.DefaultTheme())
			fixturePath := visualFixturePath(t, tc.name, "golden.png")
			if !visualgolden.Update() && !visualFixtureExists(t, fixturePath) {
				// spec-1.11 D-8: independent MARKDOWN_VISUAL_REQUIRED env
				// gate (decoupled from peer gates for spec-1.7/1.9/1.9b/1.10).
				if os.Getenv("MARKDOWN_VISUAL_REQUIRED") != "" {
					t.Fatalf("missing CC visual fixture for %s (run capture SOP first)", tc.name)
				}
				t.Skipf("missing CC visual fixture for %s; spec-1.11 capture pending (set MARKDOWN_VISUAL_REQUIRED=1 once captured)", tc.name)
			}
			visualgolden.CompareFile(t, fixturePath, png, 0.02)
		})
	}
}

// TestMarkdownVisualGolden_ParkedFixtures verifies the 5 fixture parking
// dirs exist before harness runs.
func TestMarkdownVisualGolden_ParkedFixtures(t *testing.T) {
	t.Parallel()
	required := []string{
		"MarkdownHeadings",
		"MarkdownLists",
		"MarkdownFenceCode",
		"MarkdownTable",
		"MarkdownMixed",
	}
	for _, name := range required {
		path := visualFixturePath(t, name, "")
		path = strings.TrimSuffix(path, "/")
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			t.Errorf("parked fixture dir missing: %s (run spec-1.11 capture SOP)", path)
		}
	}
}
