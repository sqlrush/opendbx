// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File markdown.go — block.Markdown production (spec-1.11 D-1 + D-2).
// 5th production block type completing block subsystem (Message + ToolUse
// + ToolResult + CompactSummary + Markdown). CC <Markdown> 等价
// (B-28 Markdown.tsx). Snapshot rendering only; streaming markdown
// (CC <StreamingMarkdown>) deferred per ❌-2.

package block

import (
	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// Markdown is the spec-1.11 5th production block type.
//
// Source is the full markdown text (caller-supplied snapshot). Per Q1
// ★A scope boundary, caller (spec-1.21) decides whether to render
// assistant content via spec-1.7 Message (plain+fence streaming path)
// or spec-1.11 Markdown (full AST snapshot path). CC dispatch (B-32)
// is: ordinary assistant text → <Markdown>; streaming → <StreamingMarkdown>;
// opendbx Stage 1 keeps Message FROZEN and lets caller choose Markdown
// for complete snapshots.
//
// No state field — LRU cache (D-4) is package-level singleton so
// Markdown struct stays value-type zero-copy.
type Markdown struct {
	Source string
}

// NewMarkdown returns a Markdown with the given source text.
func NewMarkdown(source string) Markdown { return Markdown{Source: source} }

// markdownParser is the package-level goldmark instance configured per
// Q2 ★C (CommonMark + table + linkify; no strikethrough per R3 CC
// baseline — CC disables strikethrough tokenizer). HTML raw / del nodes
// are dropped at the walker (❌-4 + I-5).
var markdownParser = goldmark.New(
	goldmark.WithExtensions(
		extension.Table,
		extension.Linkify,
	),
	goldmark.WithParserOptions(
		parser.WithAutoHeadingID(),
	),
)

// Render produces a Buffer per spec-1.11 D-2 4-step pipeline.
//
// Dispatch order (R2 HIGH-4 corrected; R3 themeKey added):
//
//  1. ctx.Cols<=0 → measureOnlyBuf(0,0) [FAST PATH 1]
//  2. cacheable := len(Source) <= markdownCacheMaxSourceBytes
//     if cacheable: cache lookup via cacheKey(Source, Cols, Verbose, themeKey)
//     hit → MeasureOnly? measureOnlyBuf(Cols, rows) : cached.buf (by reference)
//  3. miss/no-cache → goldmark.Parse + walker.walk → buffer.Buffer
//  4. MeasureOnly check → measureOnlyBuf(Cols, rows)
//     else cacheable → lru.Put + return buf
//
// Cache hit < 5µs (map lookup + return-by-reference; I-8 immutability
// contract). Cold path < 200µs (parse + walk + cell writes).
func (m Markdown) Render(ctx Context) (buffer.Buffer, error) {
	if ctx.Cols <= 0 {
		return measureOnlyBuf(0, 0), nil
	}

	theme := themeOrDefault(ctx.Theme)
	tKey := themeCacheKey(theme)

	cacheable := len(m.Source) <= markdownCacheMaxSourceBytes
	if cacheable {
		key := makeCacheKey(m.Source, ctx.Cols, ctx.Verbose, tKey)
		if cached := markdownCache.Get(key); cached != nil {
			if ctx.MeasureOnly {
				_, rows := cached.Size()
				return measureOnlyBuf(ctx.Cols, rows), nil
			}
			return cached, nil
		}
	}

	// Empty / whitespace-only source → 0 rows.
	if len(m.Source) == 0 {
		empty := measureOnlyBuf(ctx.Cols, 0)
		if cacheable {
			markdownCache.Put(makeCacheKey(m.Source, ctx.Cols, ctx.Verbose, tKey), empty)
		}
		return empty, nil
	}

	root := markdownParser.Parser().Parse(text.NewReader([]byte(m.Source)))
	buf := walkMarkdown(root, []byte(m.Source), ctx, theme)

	if ctx.MeasureOnly {
		_, rows := buf.Size()
		return measureOnlyBuf(ctx.Cols, rows), nil
	}

	if cacheable {
		markdownCache.Put(makeCacheKey(m.Source, ctx.Cols, ctx.Verbose, tKey), buf)
	}
	return buf, nil
}
