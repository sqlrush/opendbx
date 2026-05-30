// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

//go:build !windows

// Package reasoningrender_test holds the spec-1.20.2 D-6 integration
// smoke + 13th independent block-style visual fixture parking (env gate
// REASONINGRENDER_VISUAL_REQUIRED). Tests drive llmapp.Model via the
// same keystroke + Update + Cmd-chain path that program.Run uses in
// production — so a regression in the spec-1.20.2 stack (paint helpers
// landing in render, thinking control plane in llmapp, slog bridge in
// bootstrap) surfaces here, not on a user's terminal.
//
// Three production-like assertions match the spec D-6 contract:
//
//  1. **no mojibake**: CJK markdown / mixed wide+narrow text renders
//     intact (closes the 5/28 paint-pattern regression class).
//  2. **no mixed reasoning**: thinking tokens never enter the main
//     TokenStream / block.Message — strip=true drops them, strip=false
//     channels them into block.Thinking (Collapsed summary), and never
//     in the visible answer flow.
//  3. **no stderr tear**: bootstrap's slog → logger bridge keeps
//     scheduler / diagnostic warnings off the terminal grid.
package reasoningrender_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/keybindings"
	"github.com/sqlrush/opendbx/internal/app/cli/llmapp"
	"github.com/sqlrush/opendbx/internal/app/cli/program"
	"github.com/sqlrush/opendbx/internal/app/cli/render/block"
	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
	"github.com/sqlrush/opendbx/internal/app/cli/render/terminal"
	"github.com/sqlrush/opendbx/internal/domain/llm"
	"github.com/sqlrush/opendbx/internal/domain/llm/fake"
	"github.com/sqlrush/opendbx/internal/platform/logger"
)

// drive submits "ask" through a real llmapp.Model and drains the Cmd
// chain to terminal — same path as program.Run in production.
func drive(t *testing.T, prov llm.Provider, opts llmapp.Options) *llmapp.Model {
	t.Helper()
	m := llmapp.New(prov, opts)
	cur := program.Model(m)
	for _, r := range "ask" {
		cur, _ = cur.Update(program.KeyActionMsg{
			Key:    program.KeyMsg{Code: terminal.KeyRune, Rune: r},
			Action: keybindings.ActionInsertRune,
		})
	}
	cur, cmd := cur.Update(program.KeyActionMsg{
		Key:    program.KeyMsg{Code: terminal.KeyEnter},
		Action: keybindings.ActionSubmit,
	})
	for cmd != nil {
		msg := cmd()
		if msg == nil {
			break
		}
		cur, cmd = cur.Update(msg)
	}
	mm := cur.(*llmapp.Model)
	_ = mm.View(120, 30) // trigger final Drain
	return mm
}

// ============================================================
// A1 — no mojibake
// ============================================================

// TestSmoke_ChineseMarkdown_NoMojibake — a stream of Chinese markdown
// with mixed punctuation must round-trip into the rendered View intact.
// This is the production-pipeline complement to the unit-level
// paint.TestBlitAt_NoMojibakeOnMixedCJKAscii guard.
func TestSmoke_ChineseMarkdown_NoMojibake(t *testing.T) {
	t.Parallel()
	prov := fake.NewScriptedTurns(
		fake.Turn{Text: "### 中文标题\n- 一项\n- 二项", Finish: llm.FinishStop},
	)
	final := drive(t, prov, llmapp.Options{
		ModelName: "fake", MaxTokens: 1024, StripThink: true,
	})
	body := string(gridASCII(final.View(120, 30)))
	for _, want := range []string{"中文标题", "一项", "二项"} {
		if !strings.Contains(body, want) {
			t.Errorf("CJK fragment %q missing from view (mojibake?); body=\n%s", want, body)
		}
	}
}

// TestSmoke_CJKAsciiMixed_NoContinuationBleed — wide CJK runes mixed
// with narrow ASCII tokens in a single streamed line must preserve
// wide-main + continuation invariants downstream of the paint helpers.
func TestSmoke_CJKAsciiMixed_NoContinuationBleed(t *testing.T) {
	t.Parallel()
	prov := fake.NewScriptedTurns(
		fake.Turn{Text: "你 A 好 B 中 C 文", Finish: llm.FinishStop},
	)
	final := drive(t, prov, llmapp.Options{
		ModelName: "fake", MaxTokens: 1024, StripThink: true,
	})
	body := string(gridASCII(final.View(120, 30)))
	// Each wide char + its narrow neighbor must remain adjacent in
	// output order, even after blit cycles through paint.BlitAt.
	for _, want := range []string{"你 A 好 B 中 C 文"} {
		if !strings.Contains(body, want) {
			t.Errorf("CJK/ASCII mixed string scrambled; want %q in:\n%s", want, body)
		}
	}
}

// ============================================================
// A2 — no mixed reasoning
// ============================================================

// TestSmoke_NoMixedReasoning_StripTrueHidesThinking — strip=true (the
// new spec-1.20.2 D-5 default) routes thinking tokens out of the main
// content flow: View body contains the visible answer only, scrollback
// has NO block.Thinking, and the secret reasoning text never appears.
func TestSmoke_NoMixedReasoning_StripTrueHidesThinking(t *testing.T) {
	t.Parallel()
	prov := fake.New(
		llm.Chunk{Token: "secret-reasoning", Thinking: true},
		llm.Chunk{Token: "visible-answer"},
		llm.Chunk{FinishReason: llm.FinishStop},
	)
	final := drive(t, prov, llmapp.Options{
		ModelName: "fake", MaxTokens: 1024, StripThink: true,
	})
	body := string(gridASCII(final.View(120, 30)))
	if !strings.Contains(body, "visible-answer") {
		t.Errorf("visible answer missing under strip=true:\n%s", body)
	}
	if strings.Contains(body, "secret-reasoning") {
		t.Errorf("thinking token leaked into View under strip=true:\n%s", body)
	}
	for _, k := range final.ScrollbackTypesForTest() {
		if k == "block.Thinking" {
			t.Errorf("block.Thinking present under strip=true; types=%v", final.ScrollbackTypesForTest())
		}
	}
}

// TestSmoke_NoMixedReasoning_StripFalseChannelsToThinkingBlock —
// strip=false channels thinking tokens into block.Thinking (Collapsed
// summary) and NEVER into the visible answer flow. The summary string
// must surface, the raw reasoning must not pollute View as a regular
// Message line.
func TestSmoke_NoMixedReasoning_StripFalseChannelsToThinkingBlock(t *testing.T) {
	t.Parallel()
	prov := fake.New(
		llm.Chunk{Token: "secret-reasoning", Thinking: true},
		llm.Chunk{Token: "visible-answer"},
		llm.Chunk{FinishReason: llm.FinishStop},
	)
	final := drive(t, prov, llmapp.Options{
		ModelName: "fake", MaxTokens: 1024, StripThink: false,
	})
	if !hasScrollbackKind(final, "block.Thinking") {
		t.Errorf("block.Thinking missing under strip=false; types=%v", final.ScrollbackTypesForTest())
	}
	body := string(gridASCII(final.View(120, 30)))
	if !strings.Contains(body, "visible-answer") {
		t.Errorf("visible answer missing under strip=false:\n%s", body)
	}
	// The Collapsed summary is intentionally truncated (one-line "[思考中] N tokens"),
	// so the raw reasoning text MUST NOT appear in the view body — it
	// belongs inside the (collapsed) block.Thinking content only.
	if strings.Count(body, "secret-reasoning") > 0 {
		t.Errorf("raw reasoning leaked into visible View under strip=false:\n%s", body)
	}
}

// hasScrollbackKind returns true when m's scrollback contains a node of
// the named concrete type (uses the spec-1.21 test seam).
func hasScrollbackKind(m *llmapp.Model, want string) bool {
	for _, k := range m.ScrollbackTypesForTest() {
		if k == want {
			return true
		}
	}
	return false
}

// ============================================================
// A3 — no stderr tear
// ============================================================

// TestSmoke_NoStderrTear_SlogStaysOffTerminal — install the spec-1.20.2
// D-4 bridge (logger.NewSlogHandler), redirect os.Stderr to a pipe,
// fire slog.Warn (the path render/scheduler uses for frame-budget
// telemetry), restore stderr, and assert nothing landed on the
// terminal-bound writer. This is the spec D-6 invariant that closes
// the 5/28 "frame budget overshoot" tear evidence.
func TestSmoke_NoStderrTear_SlogStaysOffTerminal(t *testing.T) {
	// NOT t.Parallel — mutates os.Stderr + slog default.
	logDir := t.TempDir()
	logPath := filepath.Join(logDir, "no-tear.log")
	if err := logger.Init(logger.InitInput{SessionID: "no-tear", LogPath: logPath}); err != nil {
		t.Fatalf("logger.Init: %v", err)
	}
	t.Cleanup(func() { _ = logger.Close() })

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	oldStderr := os.Stderr
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = oldStderr })

	prevSlog := slog.Default()
	slog.SetDefault(slog.New(logger.NewSlogHandler()))
	t.Cleanup(func() { slog.SetDefault(prevSlog) })

	slog.Warn("frame budget overshoot", "elapsed", "17ms", "deadline", "16ms")
	slog.Error("BufferPool acquire failed", "size", "small")

	_ = w.Close()
	stderrBytes, _ := io.ReadAll(r)
	if len(stderrBytes) != 0 {
		t.Errorf("slog leaked to stderr (TUI tear regression): %q", stderrBytes)
	}
}

// ============================================================
// 13th visual fixture parking
// ============================================================

// TestReasoningRenderVisualGolden_ParkedFixtures parks the 13th
// independent block-style visual fixture set per spec-1.20.2 D-6.
// Each fixture dir must exist; golden.png capture lands in a follow-up
// SOP. The env gate REASONINGRENDER_VISUAL_REQUIRED=1 turns missing
// dirs into failures; missing golden.png skips until strict capture is enabled,
// matching spec-1.10..1.21 precedent.
func TestReasoningRenderVisualGolden_ParkedFixtures(t *testing.T) {
	t.Parallel()
	fixtures := []string{
		"ChineseMarkdown",
		"CJKAsciiMixed",
		"DeepSeekReasoning",
		"StripThinkOnOff",
	}
	strict := os.Getenv("REASONINGRENDER_VISUAL_REQUIRED") != ""
	for _, name := range fixtures {
		path := filepath.Join("testdata", "visual", name)
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			t.Errorf("parked fixture dir missing: %s (run spec-1.20.2 D-6 capture SOP)", path)
			continue
		}
		golden := filepath.Join(path, "golden.png")
		if _, err := os.Stat(golden); err == nil {
			continue
		}
		if strict {
			t.Errorf("missing reasoning/render golden fixture: %s", golden)
			continue
		}
		t.Skipf("missing reasoning/render golden fixture for %s; capture pending (set REASONINGRENDER_VISUAL_REQUIRED=1 once captured)", name)
	}
}

// ============================================================
// Compile-time reference to block.Thinking so a future move of the
// type out of the render/block package surfaces here, not as a runtime
// surprise on the next user's first interact session.
// ============================================================

var _ block.RenderNode = block.Thinking{}

// gridASCII renders the buffer's printable runes to a flat byte slice
// with newlines between rows. Continuation cells (the right half of a
// wide rune) are SKIPPED so a wide-main like '中' contributes exactly
// one rune to the row text instead of being trailed by a sentinel-space
// that breaks CJK substring assertions — this differs from the
// llmchat / diagnoseloop env-gate helpers which flatten continuations
// to ' ' (those tests don't assert against multi-char CJK substrings).
func gridASCII(buf buffer.Buffer) []byte {
	if buf == nil {
		return nil
	}
	cols, rows := buf.Size()
	var b bytes.Buffer
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			c := buf.Cell(x, y)
			if buffer.IsContinuation(c) {
				continue
			}
			if c.Ch <= 0 {
				b.WriteByte(' ')
				continue
			}
			b.WriteRune(c.Ch)
		}
		b.WriteByte('\n')
	}
	return b.Bytes()
}

// Suppress unused-import warning for context if a future test needs
// ctx-bearing helpers (mirrors other env gate boilerplate). Not used
// today.
var _ = context.Background
