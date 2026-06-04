// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package skills

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

// TestParse_CCGolden — the CC code-reviewer.md (survey § 3.1) parses
// byte-faithfully: 4 CC fields + block-scalar description trailing newline +
// body boundary right after the closing fence.
func TestParse_CCGolden(t *testing.T) {
	t.Parallel()
	content, err := os.ReadFile("testdata/code-reviewer.md")
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	sk, err := Parse(content, SkillSource{Kind: SourceProject, Precedence: 100, Path: "testdata/code-reviewer.md"})
	if err != nil {
		t.Fatalf("Parse golden: %v", err)
	}
	if sk.Schema.Name != "code-reviewer" {
		t.Errorf("Name = %q; want code-reviewer", sk.Schema.Name)
	}
	wantDesc := "Expert code review specialist. Proactively reviews code for quality,\n" +
		"security, and maintainability. MUST BE USED for all code changes.\n"
	if sk.Schema.Description != wantDesc {
		t.Errorf("Description = %q;\nwant %q (block-scalar trailing \\n preserved)", sk.Schema.Description, wantDesc)
	}
	if sk.Schema.Model != "sonnet" {
		t.Errorf("Model = %q; want sonnet", sk.Schema.Model)
	}
	got := sk.Schema.AllowedToolsList()
	want := []string{"Read", "Grep", "Glob", "Bash"}
	if len(got) != len(want) {
		t.Fatalf("AllowedToolsList = %v; want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("AllowedToolsList[%d] = %q; want %q", i, got[i], want[i])
		}
	}
	if len(sk.Schema.Extra) != 0 {
		t.Errorf("Extra should be empty for pure-CC frontmatter, got %v", sk.Schema.Extra)
	}
	// Body boundary: the closing fence's newline is consumed; the blank line
	// before "# Body" is part of the body (byte-faithful).
	if !strings.HasPrefix(sk.Body, "\n# Body — markdown") {
		t.Errorf("Body prefix wrong; got %q", firstN(sk.Body, 40))
	}
	if !strings.Contains(sk.Body, "Provide actionable suggestions") {
		t.Error("Body missing trailing content")
	}
	if strings.Contains(sk.Body, "name: code-reviewer") {
		t.Error("Body leaked frontmatter")
	}
}

func TestParse_NoFrontmatter(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"plain markdown": "# Just a heading\n\nbody",
		"not-a-fence":    "---foo\nname: x\n---\n",
		"empty file":     "",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := Parse([]byte(in), SkillSource{})
			assertCode(t, err, ErrNoFrontmatter.Code())
		})
	}
}

func TestParse_UnterminatedFrontmatter(t *testing.T) {
	t.Parallel()
	_, err := Parse([]byte("---\nname: x\ndescription: y\n"), SkillSource{})
	assertCode(t, err, ErrUnterminatedFrontmatter.Code())
}

// TestParse_FenceInBlockScalar — review HIGH-3: a "---" line INSIDE a block
// scalar (indented) must NOT be mistaken for the closing fence.
func TestParse_FenceInBlockScalar(t *testing.T) {
	t.Parallel()
	content := "---\n" +
		"name: tricky\n" +
		"description: |\n" +
		"  See the section below\n" +
		"  ---\n" + // indented: part of the block scalar, NOT a fence
		"  continued after a rule\n" +
		"---\n" + // column-0: the real closing fence
		"# Body\n"
	sk, err := Parse([]byte(content), SkillSource{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !strings.Contains(sk.Schema.Description, "---") {
		t.Errorf("block-scalar --- lost; Description = %q", sk.Schema.Description)
	}
	if !strings.Contains(sk.Schema.Description, "continued after a rule") {
		t.Errorf("block scalar truncated at inner ---; Description = %q", sk.Schema.Description)
	}
	if !strings.HasPrefix(sk.Body, "# Body") {
		t.Errorf("Body wrong; got %q", firstN(sk.Body, 30))
	}
}

func TestParse_BOMAndCRLF(t *testing.T) {
	t.Parallel()
	content := "\xEF\xBB\xBF---\r\nname: bom\r\ndescription: d\r\n---\r\nbody\r\n"
	sk, err := Parse([]byte(content), SkillSource{})
	if err != nil {
		t.Fatalf("Parse BOM+CRLF: %v", err)
	}
	if sk.Schema.Name != "bom" {
		t.Errorf("Name = %q; want bom", sk.Schema.Name)
	}
}

func TestParse_EmptyFrontmatter(t *testing.T) {
	t.Parallel()
	sk, err := Parse([]byte("---\n---\nbody"), SkillSource{})
	if err != nil {
		t.Fatalf("Parse empty frontmatter: %v", err)
	}
	if sk.Schema.Name != "" || len(sk.Schema.Extra) != 0 {
		t.Errorf("empty frontmatter should yield zero schema, got %+v", sk.Schema)
	}
	if sk.Body != "body" {
		t.Errorf("Body = %q; want body", sk.Body)
	}
}

func TestParse_ParseError(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		// Invalid YAML syntax (caught at yaml.Unmarshal).
		"bad syntax": "---\nname: : :\n  - broken\n---\n",
		// Valid YAML but not a mapping root (caught at root.Decode).
		"sequence not mapping": "---\n- a\n- b\n---\n",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := Parse([]byte(in), SkillSource{})
			assertCode(t, err, ErrParseError.Code())
		})
	}
}

// TestParse_ExtraDecodeError — review HIGH: an undecodable extra-field value
// (e.g. invalid !!binary) must NOT be silently dropped; Parse returns
// SKILL.PARSE_ERROR (Q6 preservation / 规则 7 non-silent).
func TestParse_ExtraDecodeError(t *testing.T) {
	t.Parallel()
	content := "---\nname: x\ndescription: d\nbad: !!binary \"not-valid-base64\"\n---\nbody\n"
	_, err := Parse([]byte(content), SkillSource{})
	assertCode(t, err, ErrParseError.Code())
}

// TestParse_FenceTrailingWhitespace — a closing fence with trailing spaces/
// tabs still closes (review: `---  ` is valid); leading whitespace does not.
func TestParse_FenceTrailingWhitespace(t *testing.T) {
	t.Parallel()
	sk, err := Parse([]byte("---\nname: x\ndescription: d\n---  \t\nbody\n"), SkillSource{})
	if err != nil {
		t.Fatalf("trailing-ws fence should close: %v", err)
	}
	if sk.Schema.Name != "x" || sk.Body != "body\n" {
		t.Errorf("unexpected parse: name=%q body=%q", sk.Schema.Name, sk.Body)
	}
}

func TestParse_TooLarge(t *testing.T) {
	t.Parallel()
	big := append([]byte("---\nname: x\ndescription: y\n---\n"), make([]byte, maxSkillSize)...)
	_, err := Parse(big, SkillSource{})
	assertCode(t, err, ErrTooLarge.Code())
}

func TestParse_TooDeep(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	b.WriteString("---\nname: deep\ndescription: d\nnested:\n")
	indent := "  "
	for i := 0; i < maxYAMLDepth+2; i++ {
		b.WriteString(strings.Repeat(indent, i+1))
		b.WriteString("k:\n")
	}
	b.WriteString("---\nbody\n")
	_, err := Parse([]byte(b.String()), SkillSource{})
	assertCode(t, err, ErrTooDeep.Code())
}

// TestParse_ExtraCollectedNotDropped — unknown frontmatter keys (incl
// opendbx_*) land in Extra; known keys never leak into Extra.
func TestParse_ExtraCollectedNotDropped(t *testing.T) {
	t.Parallel()
	content := "---\n" +
		"name: x\n" +
		"description: d\n" +
		"opendbx_databases:\n  - postgres\n  - mysql\n" +
		"future_cc_field: hello\n" +
		"---\nbody\n"
	sk, err := Parse([]byte(content), SkillSource{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if _, ok := sk.Schema.Extra["opendbx_databases"]; !ok {
		t.Errorf("opendbx_databases not preserved in Extra: %v", sk.Schema.Extra)
	}
	if _, ok := sk.Schema.Extra["future_cc_field"]; !ok {
		t.Errorf("unknown field not preserved in Extra: %v", sk.Schema.Extra)
	}
	for _, leaked := range []string{"name", "description", "allowed-tools", "model"} {
		if _, ok := sk.Schema.Extra[leaked]; ok {
			t.Errorf("known field %q leaked into Extra", leaked)
		}
	}
}

// TestKnownSchemaKeys_DriftGuard — every yaml-tagged Schema field (except the
// yaml:"-" Extra) appears in the known-key set, so Extra collection cannot
// drift from the struct definition.
func TestKnownSchemaKeys_DriftGuard(t *testing.T) {
	t.Parallel()
	known := knownSchemaKeys()
	for _, k := range []string{"name", "description", "allowed-tools", "model"} {
		if !known[k] {
			t.Errorf("known-key set missing %q (struct/known drift)", k)
		}
	}
	if known["-"] {
		t.Error("yaml:\"-\" Extra field must not be a known key")
	}
}

func firstN(s string, n int) string {
	if len(s) < n {
		return s
	}
	return s[:n]
}

func assertCode(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with code %s, got nil", want)
	}
	var ec errcode.Error
	if !errors.As(err, &ec) {
		t.Fatalf("error is not errcode.Error: %v", err)
	}
	if ec.Code() != want {
		t.Errorf("code = %s; want %s (err: %v)", ec.Code(), want, err)
	}
}
