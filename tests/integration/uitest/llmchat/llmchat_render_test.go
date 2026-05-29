// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

//go:build !windows

package llmchat_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/keybindings"
	"github.com/sqlrush/opendbx/internal/app/cli/llmapp"
	"github.com/sqlrush/opendbx/internal/app/cli/program"
	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
	"github.com/sqlrush/opendbx/internal/app/cli/render/terminal"
	"github.com/sqlrush/opendbx/internal/domain/llm"
	"github.com/sqlrush/opendbx/internal/domain/llm/fake"
	"github.com/sqlrush/opendbx/internal/testing/visualgolden"
)

// TestLLMChatVisualGolden consumes 5 LLM-chat CC fixtures parked under
// tests/integration/uitest/llmchat/testdata/visual/LLMChat*/.
//
// Fixture coverage (spec-1.20 D-10):
//
//	LLMChatStreaming — assistant text streamed into scrollback + ● indicator
//	LLMChatThinking  — thinking-only / empty → LLM.STREAM_EMPTY status (痛点 1.5)
//	LLMChatTruncated — finish_reason=length → [截断] marker
//	LLMChatToolUse   — FinishToolUse w/o Registry → DIAGNOSE.TOOL_UNKNOWN marker
//	                   (spec-1.21 T-8 retired the "[请求工具…spec-1.21]" placeholder
//	                   — Loop now emits block.ToolUse / ToolResult when a tool is
//	                   registered; this case exercises the orphan-tool fallback)
//	LLMChatError     — immediate provider error → error block
//
// 11th independent block-style visual env gate: LLMCHAT_VISUAL_REQUIRED=1
// turns missing fixtures into fatal (CI strict); without it tests t.Skip.
// Same parked-only / capture-pending pattern as spec-1.10..1.17.
func TestLLMChatVisualGolden(t *testing.T) {
	cases := []struct {
		name   string
		script *fake.Provider
		strip  bool
	}{
		{"LLMChatStreaming", fake.Scripted("Hello from Claude", llm.FinishStop), false},
		{"LLMChatThinking", fake.New(llm.Chunk{Token: "reasoning", Thinking: true}, llm.Chunk{FinishReason: llm.FinishLength}), true},
		{"LLMChatTruncated", fake.New(llm.Chunk{Token: "partial answer"}, llm.Chunk{FinishReason: llm.FinishLength}), false},
		{"LLMChatToolUse", fake.New(llm.Chunk{FinishReason: llm.FinishToolUse, ToolUses: []llm.ToolUse{{Name: "topsql"}}}), false},
		{"LLMChatError", fake.New().WithStartErr(llm.ErrAuthFailed), false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := drive(t, tc.script, tc.strip)
			grid := m.View(80, 24)
			raw := gridASCII(grid)
			fixturePath := visualFixturePath(t, tc.name, "golden.png")
			if !visualgolden.Update() && !visualFixtureExists(t, fixturePath) {
				if os.Getenv("LLMCHAT_VISUAL_REQUIRED") != "" {
					t.Fatalf("missing CC visual fixture for %s (run capture SOP first)", tc.name)
				}
				t.Skipf("missing CC visual fixture for %s; spec-1.20 capture pending (set LLMCHAT_VISUAL_REQUIRED=1 once captured)", tc.name)
			}
			png := visualgolden.Render(t, raw, visualgolden.DefaultTheme())
			visualgolden.CompareFile(t, fixturePath, png, 0.025)
		})
	}
}

// TestLLMChatVisualGolden_ParkedFixtures verifies the 5 parking dirs exist.
func TestLLMChatVisualGolden_ParkedFixtures(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"LLMChatStreaming", "LLMChatThinking", "LLMChatTruncated", "LLMChatToolUse", "LLMChatError"} {
		path := strings.TrimSuffix(visualFixturePath(t, name, ""), "/")
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			t.Errorf("parked fixture dir missing: %s (run spec-1.20 capture SOP)", path)
		}
	}
}

// drive builds an llmapp.Model with the scripted fake provider, submits a
// prompt, and synchronously runs the reader-Cmd loop to completion.
func drive(t *testing.T, p *fake.Provider, strip bool) *llmapp.Model {
	t.Helper()
	m := llmapp.New(p, llmapp.Options{ModelName: "fake-model", MaxTokens: 1024, StripThink: strip})
	cur := program.Model(m)
	for _, r := range "ask" {
		cur, _ = cur.Update(program.KeyActionMsg{Key: program.KeyMsg{Code: terminal.KeyRune, Rune: r}, Action: keybindings.ActionInsertRune})
	}
	cur, cmd := cur.Update(program.KeyActionMsg{Key: program.KeyMsg{Code: terminal.KeyEnter}, Action: keybindings.ActionSubmit})
	for cmd != nil {
		msg := cmd()
		if msg == nil {
			break
		}
		cur, cmd = cur.Update(msg)
	}
	mm := cur.(*llmapp.Model)
	// Drain in-flight render via a View pass before snapshot.
	_ = mm.View(80, 24)
	return mm
}

// --- harness helpers ---

func visualFixturePath(t testing.TB, fixture, name string) string {
	t.Helper()
	base := filepath.Join("testdata", "visual", fixture)
	if name == "" {
		return base + "/"
	}
	return filepath.Join(base, name)
}

func visualFixtureExists(t testing.TB, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	return err == nil
}

func gridASCII(buf buffer.Buffer) []byte {
	if buf == nil {
		return nil
	}
	cols, rows := buf.Size()
	var b bytes.Buffer
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			c := buf.Cell(x, y)
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
