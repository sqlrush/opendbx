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

// TestCodeVisualGolden consumes 5 Code CC fixtures parked under
// tests/integration/uitest/block/testdata/visual/Code*/.
//
// Fixture coverage (spec-1.12 D-8 § 4.2):
//
//	CodeGoFunc       — Go func with keywords + string + number
//	CodePythonScript — Python def with import + comment
//	CodeBashCommand  — Bash with $VAR + pipe + comment
//	CodeUnknownLang  — lang="zzz" → fallback plain
//	CodePlainNoLang  — lang="" → plain monospace baseline
//
// CODE_VISUAL_REQUIRED=1 turns missing fixtures into fatal (CI strict);
// without it tests t.Skip (developer-local convenience). 6th independent
// block visual env gate per spec-1.12 D-8 lesson (decoupled from
// BLOCK / TOOLUSE / TOOLRESULT / COMPACT / MARKDOWN).
func TestCodeVisualGolden(t *testing.T) {
	cases := []struct {
		name   string
		source string
		lang   string
		cols   int
	}{
		{
			name:   "CodeGoFunc",
			source: "func main() {\n    fmt.Println(\"hello\")\n    x := 42\n}",
			lang:   "go",
			cols:   80,
		},
		{
			name:   "CodePythonScript",
			source: "import os\n\n# Entry point\ndef main():\n    print('hi')\n",
			lang:   "python",
			cols:   80,
		},
		{
			name:   "CodeBashCommand",
			source: "# List files\nls -la $HOME | head -5",
			lang:   "bash",
			cols:   80,
		},
		{
			name:   "CodeUnknownLang",
			source: "no highlight here",
			lang:   "not-a-real-lang-zzz",
			cols:   80,
		},
		{
			name:   "CodePlainNoLang",
			source: "no lang specified\nplain monospace",
			lang:   "",
			cols:   80,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx := ctxDefault(tc.cols)
			c := block.NewCode(tc.source, tc.lang)
			buf, err := c.Render(ctx)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			raw := bufferANSI(t, buf)
			uiinvariant.CheckANSI(t, raw)
			png := visualgolden.Render(t, raw, visualgolden.DefaultTheme())
			fixturePath := visualFixturePath(t, tc.name, "golden.png")
			if !visualgolden.Update() && !visualFixtureExists(t, fixturePath) {
				if os.Getenv("CODE_VISUAL_REQUIRED") != "" {
					t.Fatalf("missing CC visual fixture for %s (run capture SOP first)", tc.name)
				}
				t.Skipf("missing CC visual fixture for %s; spec-1.12 capture pending (set CODE_VISUAL_REQUIRED=1 once captured)", tc.name)
			}
			visualgolden.CompareFile(t, fixturePath, png, 0.02)
		})
	}
}

// TestCodeVisualGolden_ParkedFixtures verifies the 5 fixture parking
// dirs exist before harness runs.
func TestCodeVisualGolden_ParkedFixtures(t *testing.T) {
	t.Parallel()
	required := []string{
		"CodeGoFunc",
		"CodePythonScript",
		"CodeBashCommand",
		"CodeUnknownLang",
		"CodePlainNoLang",
	}
	for _, name := range required {
		path := visualFixturePath(t, name, "")
		path = strings.TrimSuffix(path, "/")
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			t.Errorf("parked fixture dir missing: %s (run spec-1.12 capture SOP)", path)
		}
	}
}
