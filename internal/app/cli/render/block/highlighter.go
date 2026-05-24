// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File highlighter.go — spec-1.12 D-2 chroma syntax tokenize + per-
// token coarse-8 collapse + StyleEntry → style.Style mapping. Used by
// renderCodeBlock (D-3 in-place upgrade) when the active theme
// implements HighlighterTheme.
//
// Per spec § 8 Q&A:
//   - Q3 ★A: no auto-detect; `lexers.Analyse` too expensive.
//   - Q4 ★A: coarse-8 TokenType collapse (Keyword/String/Number/
//     Comment/Name/Operator/Punctuation/Code).
//   - Q5 ★D / R2 CRIT-2 ★A: color depth from ctx.ColorDepth via D-6
//     `mapChromaColor` (truecolor / 256 / 16 / 0-fallback-to-16).
//   - Q7 ★A: chroma built-in alias resolution (`golang` → go etc.).
//   - Q8 ★A: defer recover() chroma panics → return error → caller
//     falls back to plain path (I-2 preserve renderCodeBlock contract).
//
// R2 HIGH-2: `lexerProvider` interface seam for T1-25 mock injection.

package block

import (
	"errors"
	"fmt"
	"strings"

	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
	"github.com/sqlrush/opendbx/internal/app/cli/render/style"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

// lexerProvider abstracts chroma lexer lookup for test injection
// (R2 HIGH-2). Production uses realLexerProvider; T1-25 panic-mock
// substitutes a panicking implementation.
type lexerProvider interface {
	Get(name string) chroma.Lexer
	Fallback() chroma.Lexer
}

// realLexerProvider wraps chroma's package-level lexers.Get + Fallback.
type realLexerProvider struct{}

func (realLexerProvider) Get(name string) chroma.Lexer { return lexers.Get(name) }
func (realLexerProvider) Fallback() chroma.Lexer       { return lexers.Fallback }

// lexerLookup is the package-level lexer provider (Q7 ★A). Tests may
// replace via the test-only helper setLexerLookupForTest (defined in
// highlighter_test.go).
var lexerLookup lexerProvider = realLexerProvider{}

// highlightedRow is a walker-internal type carrying ready-to-paint cells
// for one rendered line. NOT exported. The renderer writes these cells
// into a buffer.Grid row.
type highlightedRow struct {
	cells []buffer.Cell
}

// highlight tokenizes body via chroma using the lexer for lang (with
// alias resolution + fallback) and returns rendered rows. The theme
// resolves chroma StyleEntry → style.Style; depth drives D-6 color
// downgrade. Returns error on chroma panic (caught via defer recover)
// or lexer.Tokenise error so the caller can fall back to plain path.
func highlight(lang, body string, theme HighlighterTheme, depth int) (rows []highlightedRow, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("chroma panic recovered: %v", r)
			rows = nil
		}
	}()

	if body == "" {
		return nil, nil
	}

	lex := resolveLexer(lang)
	if lex == nil {
		return nil, errors.New("no lexer available")
	}

	chromaStyle := resolveChromaStyle(theme)

	// chroma.Coalesce merges adjacent same-type tokens — cuts allocation
	// count significantly when the lexer emits many fine-grained tokens.
	coalesced := chroma.Coalesce(lex)
	iter, err := coalesced.Tokenise(nil, body)
	if err != nil {
		return nil, fmt.Errorf("tokenise: %w", err)
	}

	// Collect all tokens (chroma.Iterator is a function type — repeat
	// call until EOF token).
	allTokens := iterToSlice(iter)
	lines := chroma.SplitTokensIntoLines(allTokens)
	rows = make([]highlightedRow, 0, len(lines))
	for _, line := range lines {
		row := highlightedRow{cells: make([]buffer.Cell, 0, 64)}
		for _, tok := range line {
			st := chromaStyleToStyle(chromaStyle, tok.Type, depth)
			for _, r := range tok.Value {
				if r == '\n' {
					continue
				}
				row.cells = append(row.cells, buffer.Cell{Ch: r, St: st})
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// iterToSlice drains a chroma.Iterator into []chroma.Token. chroma.Iterator
// is a func type: returns the next token; an EOF token signals exhaustion.
func iterToSlice(it chroma.Iterator) []chroma.Token {
	out := make([]chroma.Token, 0, 64)
	for {
		t := it()
		if t == chroma.EOF {
			return out
		}
		out = append(out, t)
	}
}

// resolveLexer applies Q7 ★A alias resolution: chroma's built-in alias
// table handles `golang` → go etc. Empty/whitespace lang or unknown
// lang → nil (caller falls back to plain).
func resolveLexer(lang string) chroma.Lexer {
	trimmed := strings.TrimSpace(lang)
	if trimmed == "" {
		return nil
	}
	if lex := lexerLookup.Get(trimmed); lex != nil {
		return lex
	}
	// Case-insensitive retry (chroma.Get is case-sensitive for some
	// keys; alias table covers common cases).
	if lex := lexerLookup.Get(strings.ToLower(trimmed)); lex != nil {
		return lex
	}
	return nil
}

// resolveChromaStyle picks the chroma Style based on theme.CodeStyle().
// Per D-5 fallback: "" / unregistered → "monokai" (opendbx dark-default
// approximation, not CC parity claim).
func resolveChromaStyle(theme HighlighterTheme) *chroma.Style {
	if theme != nil {
		if name := theme.CodeStyle(); name != "" {
			if s := styles.Get(name); s != nil && s.Name == name {
				return s
			}
		}
	}
	if s := styles.Get("monokai"); s != nil {
		return s
	}
	return styles.Fallback
}

// chromaStyleToStyle maps a chroma.TokenType through the active
// chroma Style + D-6 color downgrade. Q4 ★A: StyleEntry attribute
// (Bold/Italic/Underline trilean) + Colour/Background → opendbx
// style.Style; coarse-8 collapse is enforced at the StyleKind layer
// elsewhere (this fn returns concrete style.Style).
func chromaStyleToStyle(cs *chroma.Style, tt chroma.TokenType, depth int) style.Style {
	if cs == nil {
		return style.Style{}
	}
	entry := cs.Get(tt)
	var st style.Style
	if entry.Colour.IsSet() {
		st.FG = mapChromaColor(entry.Colour, depth)
	}
	if entry.Background.IsSet() {
		st.BG = mapChromaColor(entry.Background, depth)
	}
	if entry.Bold == chroma.Yes {
		st.Bold = true
	}
	if entry.Italic == chroma.Yes {
		st.Italic = true
	}
	if entry.Underline == chroma.Yes {
		st.Underline = true
	}
	return st
}

// coarseStyleKindForTokenType returns the spec-1.12 Q4 ★A coarse-8
// StyleKind for a chroma TokenType. This is exposed for unit testing
// and theme integration (themes may map StyleKind back through Style()
// to influence presentation independent of chroma's per-style palette).
func coarseStyleKindForTokenType(tt chroma.TokenType) StyleKind {
	// chroma TokenType InCategory is a 1000-divisor sibling check —
	// LiteralString (3100) and LiteralNumber (3200) both share Literal
	// (3000) parent, so InCategory between siblings returns true.
	// We must check by SubCategory or numeric range. Order matters: more
	// specific check first via direct equality + SubCategory grouping.
	switch tt.SubCategory() {
	case chroma.KeywordType, chroma.Keyword:
		return StyleKeyword
	case chroma.LiteralNumber:
		return StyleNumber
	case chroma.LiteralString:
		return StyleString
	case chroma.CommentMultiline, chroma.CommentSingle, chroma.CommentPreproc, chroma.CommentSpecial, chroma.Comment:
		return StyleComment
	case chroma.NameFunction, chroma.NameClass, chroma.NameVariable, chroma.NameBuiltin, chroma.NameAttribute, chroma.Name:
		return StyleName
	case chroma.OperatorWord, chroma.Operator:
		return StyleOperator
	case chroma.Punctuation:
		return StylePunctuation
	}
	// Fallback: top-level category check.
	switch tt.Category() {
	case chroma.Keyword:
		return StyleKeyword
	case chroma.Comment:
		return StyleComment
	case chroma.Name:
		return StyleName
	case chroma.Operator:
		return StyleOperator
	case chroma.Punctuation:
		return StylePunctuation
	}
	// Literal vs Number/String distinction via numeric range.
	if int(tt) >= 3200 && int(tt) < 3300 {
		return StyleNumber
	}
	if int(tt) >= 3100 && int(tt) < 3200 {
		return StyleString
	}
	return StyleCode
}
