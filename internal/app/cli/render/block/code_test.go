// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File code_test.go — spec-1.12 D-8 unit test suite per § 4.1 (T1-1 to
// T1-33). Covers chroma lexer hit/miss/alias/fallback / token coarse-8
// mapping / color downgrade truecolor/256/16/0-fallback / renderCodeBlock
// plain vs highlight 路径 / Code{} 块 wraps renderCodeBlock / cache key
// 8-field invalidation / chroma error fallback.

package block

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/style"

	"github.com/alecthomas/chroma/v2"
)

func ctxCode(cols int) Context {
	return Context{
		Cols:       cols,
		Rows:       24,
		Wrap:       WrapSoft,
		Theme:      DefaultTheme{},
		ColorDepth: 16777216, // truecolor by default for tests; T1-14..T1-16b override
	}
}

// ---- T1-1..T1-2: edge ----

func TestCode_T1_EmptyBody(t *testing.T) {
	resetBlockCacheForTest()
	buf, err := NewCode("", "go").Render(ctxCode(80))
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	_, rows := buf.Size()
	if rows != 1 {
		t.Errorf("empty body: want 1 row (lang label only), got %d", rows)
	}
}

func TestCode_T2_PlainNoLang(t *testing.T) {
	resetBlockCacheForTest()
	buf, _ := NewCode("hello", "").Render(ctxCode(80))
	_, rows := buf.Size()
	if rows < 2 {
		t.Errorf("plain no-lang: want ≥2 rows, got %d", rows)
	}
	// Plain path uses StyleCodeBg fill (no highlight).
	wantBg := DefaultTheme{}.Style(StyleCodeBg).BG
	if got := buf.Cell(0, 1).St.BG; got != wantBg {
		t.Errorf("plain body bg: want %#v, got %#v", wantBg, got)
	}
}

// ---- T1-3..T1-5: lang-bearing fence (chroma highlight path) ----

func TestCode_T3_GoFunc(t *testing.T) {
	resetBlockCacheForTest()
	buf, _ := NewCode("func main() {}", "go").Render(ctxCode(80))
	_, rows := buf.Size()
	if rows < 2 {
		t.Fatalf("go func: want ≥2 rows, got %d", rows)
	}
	// At least one cell on body row should have non-zero FG (highlighted).
	hasHighlight := false
	for x := 1; x < 30; x++ {
		if buf.Cell(x, 1).St.FG != 0 {
			hasHighlight = true
			break
		}
	}
	if !hasHighlight {
		t.Errorf("go func: want highlighted cells with non-zero FG on body row")
	}
}

func TestCode_T4_PythonDef(t *testing.T) {
	resetBlockCacheForTest()
	buf, _ := NewCode("def x(): pass", "python").Render(ctxCode(80))
	_, rows := buf.Size()
	if rows < 2 {
		t.Errorf("python def: want ≥2 rows, got %d", rows)
	}
}

func TestCode_T5_BashCmd(t *testing.T) {
	resetBlockCacheForTest()
	buf, _ := NewCode("ls -la", "bash").Render(ctxCode(80))
	_, rows := buf.Size()
	if rows < 2 {
		t.Errorf("bash cmd: want ≥2 rows, got %d", rows)
	}
}

// ---- T1-6..T1-10: lang resolve / alias / fallback ----

func TestCode_T6_UnknownLangFallback(t *testing.T) {
	resetBlockCacheForTest()
	// Unknown lang → resolveLexer returns nil → highlight returns
	// error → renderCodeBlock falls back to plain path.
	buf, _ := NewCode("x", "not-a-lang-zzz").Render(ctxCode(80))
	_, rows := buf.Size()
	if rows < 2 {
		t.Errorf("unknown lang: want ≥2 rows (plain path), got %d", rows)
	}
}

func TestCode_T7_LangAliasResolve(t *testing.T) {
	if lex := resolveLexer("golang"); lex == nil {
		t.Errorf("alias resolve 'golang' → go: want non-nil lexer, got nil")
	}
}

func TestCode_T8_LangCaseInsensitive(t *testing.T) {
	for _, name := range []string{"Go", "GO", "go"} {
		if lex := resolveLexer(name); lex == nil {
			t.Errorf("case-insensitive %q: want non-nil lexer, got nil", name)
		}
	}
}

func TestCode_T9_EmptyLang(t *testing.T) {
	if lex := resolveLexer(""); lex != nil {
		t.Errorf("empty lang: want nil lexer, got %v", lex)
	}
}

func TestCode_T10_WhitespaceOnlyLang(t *testing.T) {
	if lex := resolveLexer("   "); lex != nil {
		t.Errorf("whitespace lang: want nil lexer, got %v", lex)
	}
}

// ---- T1-11..T1-13: theme + style fallback ----

// styleOnlyTheme implements StyleTheme without HighlighterTheme.
type styleOnlyTheme struct{}

func (styleOnlyTheme) Style(k StyleKind) style.Style { return DefaultTheme{}.Style(k) }

func TestCode_T11_ThemeWithoutHighlighterTheme(t *testing.T) {
	resetBlockCacheForTest()
	ctx := ctxCode(80)
	ctx.Theme = styleOnlyTheme{}
	buf, _ := NewCode("func main() {}", "go").Render(ctx)
	// Without HighlighterTheme → plain path → uniform StyleCodeBg fill.
	wantBg := DefaultTheme{}.Style(StyleCodeBg).BG
	if got := buf.Cell(0, 1).St.BG; got != wantBg {
		t.Errorf("plain path (no HighlighterTheme): want StyleCodeBg fill, got %#v", got)
	}
}

// emptyCodeStyleTheme returns "" → fallback to monokai.
type emptyCodeStyleTheme struct{ DefaultTheme }

func (emptyCodeStyleTheme) CodeStyle() string { return "" }

func TestCode_T12_CodeStyleReturnsEmpty(t *testing.T) {
	if s := resolveChromaStyle(emptyCodeStyleTheme{}); s.Name != "monokai" {
		t.Errorf("empty CodeStyle: want fallback to 'monokai', got %q", s.Name)
	}
}

// badCodeStyleTheme returns unregistered name → fallback to monokai.
type badCodeStyleTheme struct{ DefaultTheme }

func (badCodeStyleTheme) CodeStyle() string { return "no-such-style-zzz" }

func TestCode_T13_CodeStyleReturnsUnregistered(t *testing.T) {
	if s := resolveChromaStyle(badCodeStyleTheme{}); s.Name != "monokai" {
		t.Errorf("unregistered CodeStyle: want fallback to 'monokai', got %q", s.Name)
	}
}

// ---- T1-14..T1-16b: color depth downgrade ----

func TestCode_T14_Truecolor(t *testing.T) {
	c := chroma.NewColour(0xff, 0x80, 0x40)
	got := mapChromaColor(c, 16777216)
	want := style.RGB(0xff, 0x80, 0x40)
	if got != want {
		t.Errorf("truecolor RGB: want %#v, got %#v", want, got)
	}
}

func TestCode_T15_Downgrade256(t *testing.T) {
	c := chroma.NewColour(0xff, 0x80, 0x40)
	got256 := mapChromaColor(c, 256)
	got16 := mapChromaColor(c, 16)
	// 256 path produces a different (more precise) palette index than 16.
	if got256 == got16 {
		t.Errorf("256 downgrade should be more precise than 16-color; got identical %#x", got256)
	}
	// Both should be non-zero (mapped to a palette entry).
	if got256 == 0 {
		t.Errorf("256 downgrade: want non-zero palette color, got 0")
	}
}

func TestCode_T16a_Downgrade16(t *testing.T) {
	c := chroma.NewColour(0xff, 0x00, 0x00)
	got := mapChromaColor(c, 16)
	// Bright red ≈ palette index 9.
	if got == 0 {
		t.Errorf("16 downgrade: want non-zero, got 0")
	}
}

func TestCode_T16b_ZeroDepthConservativeFallback(t *testing.T) {
	c := chroma.NewColour(0xff, 0x00, 0x00)
	got16 := mapChromaColor(c, 16)
	got0 := mapChromaColor(c, 0)
	// R3 HIGH-2: ColorDepth=0 follows conservative 16-color fallback.
	if got0 != got16 {
		t.Errorf("zero-depth conservative fallback (R3 HIGH-2): want same as 16, got %#x vs %#x", got0, got16)
	}
}

// ---- T1-17..T1-24: TokenType coarse-8 collapse ----

func TestCode_T17_TokenCoarseKeyword(t *testing.T) {
	if got := coarseStyleKindForTokenType(chroma.Keyword); got != StyleKeyword {
		t.Errorf("Keyword: want StyleKeyword, got %v", got)
	}
}

func TestCode_T18_TokenCoarseString(t *testing.T) {
	if got := coarseStyleKindForTokenType(chroma.LiteralString); got != StyleString {
		t.Errorf("LiteralString: want StyleString, got %v", got)
	}
}

func TestCode_T19_TokenCoarseNumber(t *testing.T) {
	if got := coarseStyleKindForTokenType(chroma.LiteralNumber); got != StyleNumber {
		t.Errorf("LiteralNumber: want StyleNumber, got %v", got)
	}
}

func TestCode_T20_TokenCoarseComment(t *testing.T) {
	if got := coarseStyleKindForTokenType(chroma.Comment); got != StyleComment {
		t.Errorf("Comment: want StyleComment, got %v", got)
	}
}

func TestCode_T21_TokenCoarseName(t *testing.T) {
	if got := coarseStyleKindForTokenType(chroma.Name); got != StyleName {
		t.Errorf("Name: want StyleName, got %v", got)
	}
}

func TestCode_T22_TokenCoarseOperator(t *testing.T) {
	if got := coarseStyleKindForTokenType(chroma.Operator); got != StyleOperator {
		t.Errorf("Operator: want StyleOperator, got %v", got)
	}
}

func TestCode_T23_TokenCoarsePunctuation(t *testing.T) {
	if got := coarseStyleKindForTokenType(chroma.Punctuation); got != StylePunctuation {
		t.Errorf("Punctuation: want StylePunctuation, got %v", got)
	}
}

func TestCode_T24_TokenCoarseFallback(t *testing.T) {
	if got := coarseStyleKindForTokenType(chroma.Background); got != StyleCode {
		t.Errorf("Background default: want StyleCode, got %v", got)
	}
}

// ---- T1-25: chroma error fallback (lexerProvider panic-mock injection) ----

type panickingProvider struct{}

func (panickingProvider) Get(name string) chroma.Lexer { panic("chroma test panic") }
func (panickingProvider) Fallback() chroma.Lexer       { panic("chroma test panic") }

func TestCode_T25_ChromaErrorFallback(t *testing.T) {
	resetBlockCacheForTest()
	original := lexerLookup
	lexerLookup = panickingProvider{}
	defer func() { lexerLookup = original }()

	buf, err := NewCode("hello", "go").Render(ctxCode(80))
	if err != nil {
		t.Fatalf("Render returned err (should fallback silently): %v", err)
	}
	_, rows := buf.Size()
	if rows < 2 {
		t.Errorf("chroma panic fallback: want ≥2 rows from plain path, got %d", rows)
	}
}

// ---- T1-26..T1-27: long source + CJK ----

func TestCode_T26_LongSource(t *testing.T) {
	resetBlockCacheForTest()
	long := strings.Repeat("var x = 1\n", 500)
	buf, _ := NewCode(long, "go").Render(ctxCode(80))
	_, rows := buf.Size()
	if rows < 100 {
		t.Errorf("long source 500-line: want ≥100 rows, got %d", rows)
	}
}

func TestCode_T27_CJKInString(t *testing.T) {
	resetBlockCacheForTest()
	buf, _ := NewCode(`s := "中文"`, "go").Render(ctxCode(80))
	_, rows := buf.Size()
	if rows < 2 {
		t.Errorf("CJK in string: want ≥2 rows, got %d", rows)
	}
}

// ---- T1-28..T1-30: cache key 8-field invalidation ----

func TestCode_T28_CacheHitReturnsSameBuffer(t *testing.T) {
	resetBlockCacheForTest()
	c := NewCode("var x = 1", "go")
	buf1, _ := c.Render(ctxCode(80))
	buf2, _ := c.Render(ctxCode(80))
	if fmt.Sprintf("%p", buf1) != fmt.Sprintf("%p", buf2) {
		t.Errorf("cache hit (I-8): want same Buffer ref, got different")
	}
}

func TestCode_T29_CacheMissOnDifferentLang(t *testing.T) {
	resetBlockCacheForTest()
	buf1, _ := NewCode("x", "go").Render(ctxCode(80))
	buf2, _ := NewCode("x", "python").Render(ctxCode(80))
	if fmt.Sprintf("%p", buf1) == fmt.Sprintf("%p", buf2) {
		t.Errorf("different lang: want different cache entry, got same Buffer")
	}
}

// codeStyleTestTheme implements HighlighterTheme with configurable style.
type codeStyleTestTheme struct {
	DefaultTheme
	styleName string
}

func (t codeStyleTestTheme) CodeStyle() string { return t.styleName }

func TestCode_T30a_CacheMissOnDifferentCodeStyle(t *testing.T) {
	resetBlockCacheForTest()
	ctx := ctxCode(80)
	ctx.Theme = codeStyleTestTheme{styleName: "monokai"}
	buf1, _ := NewCode("x", "go").Render(ctx)
	ctx.Theme = codeStyleTestTheme{styleName: "github"}
	buf2, _ := NewCode("x", "go").Render(ctx)
	if fmt.Sprintf("%p", buf1) == fmt.Sprintf("%p", buf2) {
		t.Errorf("different codeStyle: want different cache entry, got same Buffer")
	}
}

func TestCode_T30b_CacheMissOnDifferentColorDepth(t *testing.T) {
	resetBlockCacheForTest()
	ctx1 := ctxCode(80)
	ctx1.ColorDepth = 16
	ctx2 := ctxCode(80)
	ctx2.ColorDepth = 256
	buf1, _ := NewCode("x", "go").Render(ctx1)
	buf2, _ := NewCode("x", "go").Render(ctx2)
	if fmt.Sprintf("%p", buf1) == fmt.Sprintf("%p", buf2) {
		t.Errorf("different colorDepth: want different cache entry, got same Buffer")
	}
}

// ---- T1-31: Code{} block wraps renderCodeBlock ----

func TestCode_T31_CodeBlockWrapsRenderCodeBlock(t *testing.T) {
	resetBlockCacheForTest()
	bufA, _ := NewCode("var x = 1", "go").Render(ctxCode(80))
	// Direct renderCodeBlock should produce the same Buffer reference
	// (cache hit via same 8-field key).
	bufB, _ := renderCodeBlock(ctxCode(80), "go", "var x = 1")
	if fmt.Sprintf("%p", bufA) != fmt.Sprintf("%p", bufB) {
		t.Errorf("Code{} wraps renderCodeBlock: want same Buffer ref via cache hit, got different")
	}
}

// ---- T1-36: concurrent Render race ----

// TestCode_R2_NilThemeAutoHighlight — R2 codex CRIT-1 regression:
// nil Theme should be normalized to DefaultTheme per § 3.2 + I-3, and
// DefaultTheme implements HighlighterTheme per CRIT-1 ★B, so lang-
// bearing fences with nil Theme should trigger highlight (not silently
// fall back to plain path).
func TestCode_R2_NilThemeAutoHighlight(t *testing.T) {
	resetBlockCacheForTest()
	c := NewCode("func main() {}", "go")
	buf, err := c.Render(Context{Cols: 80}) // nil Theme
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	hasHighlight := false
	for x := 1; x < 30; x++ {
		if buf.Cell(x, 1).St.FG != 0 {
			hasHighlight = true
			break
		}
	}
	if !hasHighlight {
		t.Errorf("nil Theme (default-on highlight per CRIT-1 ★B): want highlighted cells, got plain")
	}
}

// TestMarkdown_R2_NilThemeFenceAutoHighlight — R2 codex CRIT-1 same
// invariant via Markdown caller (which leaves Theme nil by default).
func TestMarkdown_R2_NilThemeFenceAutoHighlight(t *testing.T) {
	resetBlockCacheForTest()
	m := NewMarkdown("```go\nfunc main() {}\n```")
	buf, err := m.Render(Context{Cols: 80, Wrap: WrapSoft}) // nil Theme
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	// Row 0 is the lang label, body starts row 1.
	hasHighlight := false
	for x := 1; x < 30; x++ {
		if buf.Cell(x, 1).St.FG != 0 {
			hasHighlight = true
			break
		}
	}
	if !hasHighlight {
		t.Errorf("Markdown nil Theme fence (default-on highlight per CRIT-1 ★B): want highlighted cells, got plain")
	}
}

// TestMarkdown_R2_CacheMissOnDifferentCodeStyle — R2 codex HIGH-1
// regression: Markdown body containing fenced code blocks must
// invalidate cache when theme.CodeStyle() changes.
func TestMarkdown_R2_CacheMissOnDifferentCodeStyle(t *testing.T) {
	resetBlockCacheForTest()
	src := "```go\nfunc main() {}\n```"
	ctx1 := Context{Cols: 80, Wrap: WrapSoft, Theme: codeStyleTestTheme{styleName: "monokai"}}
	ctx2 := Context{Cols: 80, Wrap: WrapSoft, Theme: codeStyleTestTheme{styleName: "github"}}
	buf1, _ := NewMarkdown(src).Render(ctx1)
	buf2, _ := NewMarkdown(src).Render(ctx2)
	if fmt.Sprintf("%p", buf1) == fmt.Sprintf("%p", buf2) {
		t.Errorf("Markdown fence with different CodeStyle: want different cache entry (HIGH-1), got same Buffer ref")
	}
}

func TestCode_ConcurrentRenderRace(t *testing.T) {
	resetBlockCacheForTest()
	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				c := NewCode(fmt.Sprintf("var x%d = %d", seed, i), "go")
				_, _ = c.Render(ctxCode(80))
			}
		}(g)
	}
	wg.Wait()
}
