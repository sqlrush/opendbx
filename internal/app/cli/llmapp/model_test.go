// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package llmapp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/sqlrush/opendbx/internal/app/cli/keybindings"
	"github.com/sqlrush/opendbx/internal/app/cli/program"
	"github.com/sqlrush/opendbx/internal/app/cli/render/block"
	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
	"github.com/sqlrush/opendbx/internal/app/cli/render/streaming"
	"github.com/sqlrush/opendbx/internal/app/cli/render/terminal"
	"github.com/sqlrush/opendbx/internal/app/diagnose"
	"github.com/sqlrush/opendbx/internal/domain/llm"
	"github.com/sqlrush/opendbx/internal/domain/llm/fake"
)

func keyAction(code int, r rune) program.KeyActionMsg {
	k := program.KeyMsg{Code: code, Rune: r}
	return program.KeyActionMsg{Key: k, Action: keybindings.Resolve(keybindings.KeyEvent{Code: code, Rune: r})}
}

func typeAndModel(t *testing.T, m *Model, text string) *Model {
	t.Helper()
	cur := program.Model(m)
	for _, r := range text {
		next, _ := cur.Update(keyAction(terminal.KeyRune, r))
		cur = next
	}
	return cur.(*Model)
}

// runStream drives the full reader-Cmd loop synchronously with a fake
// provider (no goroutine timing): submit → run startCmd → loop
// readControlMsg → Cmd → control/done until idle. Returns the final Model.
func runStream(t *testing.T, m *Model) *Model {
	t.Helper()
	cur := program.Model(m)
	mm, cmd := cur.Update(keyAction(terminal.KeyEnter, 0))
	cur = mm
	// Drive the Cmd chain until no Cmd is returned (stream fully drained).
	for cmd != nil {
		msg := cmd()
		if msg == nil {
			break
		}
		cur, cmd = cur.Update(msg)
	}
	return cur.(*Model)
}

func newFakeModel(p llm.Provider) *Model {
	return New(p, Options{ModelName: "fake-model", MaxTokens: 1024})
}

func TestModel_InputEditing(t *testing.T) {
	t.Parallel()
	m := typeAndModel(t, newFakeModel(fake.New()), "hello")
	if st := m.InputState(); st.Buffer != "hello" || st.Cursor != 5 {
		t.Errorf("InputState = %+v; want hello/5", st)
	}
}

func TestModel_SubmitClearsBuffer(t *testing.T) {
	t.Parallel()
	m := typeAndModel(t, newFakeModel(fake.Scripted("hi there", llm.FinishStop)), "question")
	final := runStream(t, m)
	if final.InputState().Buffer != "" {
		t.Errorf("buffer not cleared after submit: %q", final.InputState().Buffer)
	}
	if final.streaming {
		t.Errorf("still streaming after drain; want idle")
	}
	// scrollback has the user echo + assistant text drained.
	if len(final.scrollback) == 0 {
		t.Errorf("scrollback empty after stream")
	}
}

func TestModel_SubmitEmptyNoOp(t *testing.T) {
	t.Parallel()
	m := newFakeModel(fake.Scripted("x", llm.FinishStop))
	next, cmd := m.Update(keyAction(terminal.KeyEnter, 0))
	if cmd != nil {
		t.Errorf("empty submit should return nil cmd")
	}
	if next.(*Model).streaming {
		t.Errorf("empty submit should not start streaming")
	}
}

func TestModel_StreamingBlocksInput(t *testing.T) {
	t.Parallel()
	// While streaming, rune input is ignored (single-turn).
	m := newFakeModel(fake.New(llm.Chunk{Token: "a"})) // no finish → stays "streaming" mid-flight in model
	m = typeAndModel(t, m, "q")
	mm, _ := m.Update(keyAction(terminal.KeyEnter, 0))
	streamingModel := mm.(*Model)
	if !streamingModel.streaming {
		t.Fatalf("expected streaming=true after submit")
	}
	after, _ := streamingModel.Update(keyAction(terminal.KeyRune, 'z'))
	if after.(*Model).InputState().Buffer != "" {
		t.Errorf("input during streaming should be ignored; buffer=%q", after.(*Model).InputState().Buffer)
	}
}

// TestModel_SawContent_TextVsThinking is the R2.2 HIGH-2 regression:
// text + FinishLength must NOT trigger STREAM_EMPTY (sawContent=true),
// while thinking-only + FinishLength MUST (sawContent=false).
func TestModel_SawContent_TextThenLength(t *testing.T) {
	t.Parallel()
	m := typeAndModel(t, newFakeModel(fake.New(
		llm.Chunk{Token: "partial answer"},
		llm.Chunk{FinishReason: llm.FinishLength},
	)), "q")
	final := runStream(t, m)
	if hasNode(final, "STREAM_EMPTY") {
		t.Errorf("text+Length wrongly produced STREAM_EMPTY")
	}
	if !hasNode(final, "截断") {
		t.Errorf("text+Length should show truncation marker")
	}
}

func TestModel_SawContent_ThinkingOnlyLength(t *testing.T) {
	t.Parallel()
	m := typeAndModel(t, newFakeModel(fake.New(
		llm.Chunk{Token: "reasoning", Thinking: true},
		llm.Chunk{FinishReason: llm.FinishLength},
	)).withStripThink(true), "q")
	final := runStream(t, m)
	if !hasNode(final, "STREAM_EMPTY") {
		t.Errorf("thinking-only+Length should produce STREAM_EMPTY status")
	}
}

func TestModel_ThinkingStripTrueDoesNotPolluteMainContent(t *testing.T) {
	t.Parallel()
	m := typeAndModel(t, newFakeModel(fake.New(
		llm.Chunk{Token: "reasoning secret", Thinking: true},
		llm.Chunk{Token: "visible answer"},
		llm.Chunk{FinishReason: llm.FinishStop},
	)).withStripThink(true), "q")
	final := runStream(t, m)
	if hasThinkingBlock(final) {
		t.Fatalf("strip_think=true should not render block.Thinking; nodes=%v", final.ScrollbackTypesForTest())
	}
	if messageNodeContains(final, "reasoning secret") {
		t.Fatalf("thinking token leaked into block.Message main content: %v", nodeTexts(final))
	}
	if !hasNode(final, "visible answer") {
		t.Fatalf("visible answer missing: %v", nodeTexts(final))
	}
}

func TestModel_ThinkingStripFalseRendersThinkingBlock(t *testing.T) {
	t.Parallel()
	m := typeAndModel(t, newFakeModel(fake.New(
		llm.Chunk{Token: "reasoning secret", Thinking: true},
		llm.Chunk{Token: "visible answer"},
		llm.Chunk{FinishReason: llm.FinishStop},
	)).withStripThink(false), "q")
	final := runStream(t, m)
	if !hasThinkingBlock(final) {
		t.Fatalf("strip_think=false should render block.Thinking; nodes=%v", final.ScrollbackTypesForTest())
	}
	if !thinkingBlockContains(final, "reasoning secret") {
		t.Fatalf("block.Thinking missing reasoning content")
	}
	if messageNodeContains(final, "reasoning secret") {
		t.Fatalf("thinking token leaked into block.Message main content: %v", nodeTexts(final))
	}
	if !hasNode(final, "visible answer") {
		t.Fatalf("visible answer missing: %v", nodeTexts(final))
	}
}

func TestModel_ThinkingOnlyStopStripTrueMarker(t *testing.T) {
	t.Parallel()
	m := typeAndModel(t, newFakeModel(fake.New(
		llm.Chunk{Token: "reasoning only", Thinking: true},
		llm.Chunk{FinishReason: llm.FinishStop},
	)).withStripThink(true), "q")
	final := runStream(t, m)
	if !hasNode(final, block.ThinkingOnlyStripMarker()) {
		t.Fatalf("thinking-only strip=true marker missing: %v", nodeTexts(final))
	}
	if hasNode(final, "(no output)") {
		t.Fatalf("thinking-only strip=true should not show generic empty placeholder: %v", nodeTexts(final))
	}
}

func TestModel_ThinkingOnlyStopStripFalseThinkingBlock(t *testing.T) {
	t.Parallel()
	m := typeAndModel(t, newFakeModel(fake.New(
		llm.Chunk{Token: "reasoning only", Thinking: true},
		llm.Chunk{FinishReason: llm.FinishStop},
	)).withStripThink(false), "q")
	final := runStream(t, m)
	if !hasThinkingBlock(final) || !thinkingBlockContains(final, "reasoning only") {
		t.Fatalf("thinking-only strip=false should render block.Thinking; nodes=%v", final.ScrollbackTypesForTest())
	}
	if hasNode(final, "(no output)") {
		t.Fatalf("thinking-only strip=false should not show generic empty placeholder: %v", nodeTexts(final))
	}
}

func TestModel_ViewFiltersThinkingOnlyEmptyBeforeStreamDone(t *testing.T) {
	t.Parallel()
	m := newFakeModel(fake.New()).withStripThink(true)
	m.stream = streaming.NewTokenStream(context.Background())
	m.sawThinking = true
	m.sawContent = false
	m.scrollback = appendNode(m.scrollback, block.Message{Text: block.ThinkingOnlyStripMarker()})
	if err := m.stream.AppendChunk(streaming.Chunk{FinishReason: streaming.FinishStop}); err != nil {
		t.Fatalf("AppendChunk: %v", err)
	}

	_ = m.View(80, 24)
	if hasNode(m, "(no output)") {
		t.Fatalf("View drain leaked generic empty placeholder before streamDone: %v", nodeTexts(m))
	}
	if !hasNode(m, block.ThinkingOnlyStripMarker()) {
		t.Fatalf("thinking-only marker missing after View drain: %v", nodeTexts(m))
	}
}

func TestModel_ImmediateError(t *testing.T) {
	t.Parallel()
	m := typeAndModel(t, newFakeModel(fake.New().WithStartErr(llm.ErrAuthFailed)), "q")
	final := runStream(t, m)
	if !hasNode(final, "错误") {
		t.Errorf("immediate auth error should render an error node; scrollback=%v", nodeTexts(final))
	}
	if final.streaming {
		t.Errorf("should be idle after immediate error")
	}
}

// TestModel_LoopUnknownTool exercises the spec-1.21 D-6 TOOL_UNKNOWN
// surface: a provider that emits FinishToolUse against a Model with no
// Registry (or one missing the tool) renders the DIAGNOSE.TOOL_UNKNOWN
// terminal marker — never the retired "spec-1.21" placeholder.
func TestModel_LoopUnknownTool(t *testing.T) {
	t.Parallel()
	m := typeAndModel(t, newFakeModel(fake.New(
		llm.Chunk{FinishReason: llm.FinishToolUse, ToolUses: []llm.ToolUse{{ID: "c1", Name: "topsql"}}},
	)), "q")
	final := runStream(t, m)
	if !hasNode(final, "DIAGNOSE.TOOL_UNKNOWN") {
		t.Errorf("unknown tool should render DIAGNOSE.TOOL_UNKNOWN marker; got %v", nodeTexts(final))
	}
	// And the retired placeholder text must NOT leak.
	if hasNode(final, "spec-1.21") {
		t.Errorf("retired placeholder leaked into scrollback: %v", nodeTexts(final))
	}
}

// TestModel_LoopTransitionsToolUseState is the codex T-10a P2-1 absorb:
// once a tool finishes, the UI MUST reflect Resolved/Error on the
// matching block.ToolUse — otherwise a user sees the tool stuck on
// "Running" even though its result is already rendered below.
func TestModel_LoopTransitionsToolUseState(t *testing.T) {
	t.Parallel()
	reg, _ := diagnose.NewRegistry(diagnose.EchoTool{})
	prov := fake.NewScriptedTurns(
		fake.Turn{ToolUses: []llm.ToolUse{{ID: "c1", Name: "echo"}}, Finish: llm.FinishToolUse},
		fake.Turn{Text: "done", Finish: llm.FinishStop},
	)
	m := New(prov, Options{ModelName: "fake", MaxTokens: 1024, Registry: reg})
	m = typeAndModel(t, m, "x")
	final := runStream(t, m)

	// Find the (single) block.ToolUse in scrollback.
	var found *block.ToolUse
	for _, n := range final.scrollback {
		if tu, ok := n.(block.ToolUse); ok {
			tu := tu
			found = &tu
			break
		}
	}
	if found == nil {
		t.Fatalf("no block.ToolUse in scrollback; nodes = %v", nodeTexts(final))
	}
	if found.State != block.StateResolved {
		t.Errorf("ToolUse.State = %v; want StateResolved (codex P2-1 — tool finished, UI must not show Running)", found.State)
	}
}

// TestModel_LoopTransitionsToolUseToError mirrors P2-1 for the IsError
// path: a tool reporting recoverable failure (IsError=true) must mark
// the matching ToolUse as StateError so the UI shows a failure marker
// rather than "Running" or a success tick.
func TestModel_LoopTransitionsToolUseToError(t *testing.T) {
	t.Parallel()
	reg, _ := diagnose.NewRegistry(errToolModel{})
	prov := fake.NewScriptedTurns(
		fake.Turn{ToolUses: []llm.ToolUse{{ID: "c1", Name: "fail-test"}}, Finish: llm.FinishToolUse},
		fake.Turn{Text: "ok", Finish: llm.FinishStop},
	)
	m := New(prov, Options{ModelName: "fake", MaxTokens: 1024, Registry: reg})
	m = typeAndModel(t, m, "x")
	final := runStream(t, m)

	var found *block.ToolUse
	for _, n := range final.scrollback {
		if tu, ok := n.(block.ToolUse); ok {
			tu := tu
			found = &tu
			break
		}
	}
	if found == nil {
		t.Fatalf("no block.ToolUse in scrollback")
	}
	if found.State != block.StateError {
		t.Errorf("ToolUse.State = %v; want StateError (IsError result path)", found.State)
	}
}

// errToolModel is a minimal ToolExecutor that always returns IsError=true.
type errToolModel struct{}

func (errToolModel) Name() string { return "fail-test" }
func (errToolModel) Schema() llm.ToolSchema {
	return llm.ToolSchema{Name: "fail-test", InputSchema: map[string]any{"type": "object"}}
}
func (errToolModel) Execute(context.Context, map[string]any) (diagnose.ToolOutput, error) {
	return diagnose.ToolOutput{Content: "boom", IsError: true}, nil
}

// TestModel_LoopAppendsToolBlocks is the headline T-8 case: a Loop with
// a registered tool runs through user → tool_use → tool_result → final
// text, and the scrollback ends up holding block.ToolUse + block.ToolResult
// (NOT placeholders) plus the assistant prose. Asserts the spec-1.21 D-6
// adapter wiring: makeEmit / handleControl / appendNode → render nodes
// of the right types in the right order.
func TestModel_LoopAppendsToolBlocks(t *testing.T) {
	t.Parallel()
	reg, err := diagnose.NewRegistry(diagnose.EchoTool{})
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	prov := fake.NewScriptedTurns(
		fake.Turn{
			ToolUses: []llm.ToolUse{{ID: "c1", Name: "echo", Input: map[string]any{"msg": "ping"}}},
			Finish:   llm.FinishToolUse,
		},
		fake.Turn{Text: "all done", Finish: llm.FinishStop},
	)
	m := New(prov, Options{ModelName: "loop", MaxTokens: 1024, Registry: reg})
	m = typeAndModel(t, m, "go")
	final := runStream(t, m)

	if final.streaming {
		t.Errorf("still streaming after drain")
	}
	// Walk scrollback nodes: expect [user msg, block.ToolUse, block.ToolResult, assistant text].
	if len(final.scrollback) < 4 {
		t.Fatalf("scrollback too short (%d nodes): %v", len(final.scrollback), final.scrollback)
	}
	if _, ok := final.scrollback[1].(block.ToolUse); !ok {
		t.Errorf("scrollback[1] = %T; want block.ToolUse (spec-1.21 D-6 retire placeholder)", final.scrollback[1])
	}
	if _, ok := final.scrollback[2].(block.ToolResult); !ok {
		t.Errorf("scrollback[2] = %T; want block.ToolResult", final.scrollback[2])
	}
	if !hasNode(final, "all done") {
		t.Errorf("assistant final text not drained into scrollback: %v", nodeTexts(final))
	}
}

func TestModel_StatusSegments(t *testing.T) {
	t.Parallel()
	m := newFakeModel(fake.New())
	segs := m.StatusSegments()
	if len(segs) == 0 || segs[0].Text != "fake-model" {
		t.Errorf("status = %+v; want [fake-model]", segs)
	}
}

func TestModel_HistoryBounded(t *testing.T) {
	t.Parallel()
	m := New(fake.Scripted("ok", llm.FinishStop), Options{MaxHistory: 3, MaxTokens: 100})
	cur := m
	for i := 0; i < 6; i++ {
		cur = typeAndModel(t, cur, "msg")
		cur = runStream(t, cur)
	}
	if len(cur.history) > 3 {
		t.Errorf("history len = %d; want ≤ 3 (bounded)", len(cur.history))
	}
}

// TestModel_AssistantTextDrained is the T-10a HIGH-3 regression: the
// assistant text must reach scrollback even though the final Drain happens
// only after the stream is Closed. runStream never calls View, so this
// fails if the streamDoneMsg handler does not Drain before nil-ing stream.
func TestModel_AssistantTextDrained(t *testing.T) {
	t.Parallel()
	m := typeAndModel(t, newFakeModel(fake.Scripted("hello world", llm.FinishStop)), "q")
	final := runStream(t, m)
	if !hasNode(final, "hello world") {
		t.Errorf("assistant text not drained into scrollback (HIGH-3); got %v", nodeTexts(final))
	}
}

// captureProvider records the ctx passed to Stream and returns a stream
// that blocks until that ctx is cancelled — used to assert that submit
// threads the cancel ctx through to the provider (T-10a HIGH-1).
type captureProvider struct{ gotCtx chan context.Context }

func (p *captureProvider) Name() string { return "capture" }
func (p *captureProvider) Stream(ctx context.Context, _ llm.Request) (llm.Stream, error) {
	p.gotCtx <- ctx
	return &blockingStream{ctx: ctx}, nil
}

type blockingStream struct {
	ctx context.Context
	err error
}

func (s *blockingStream) Next() bool {
	<-s.ctx.Done()
	s.err = s.ctx.Err()
	return false
}
func (s *blockingStream) Chunk() llm.Chunk { return llm.Chunk{} }
func (s *blockingStream) Err() error       { return s.err }
func (s *blockingStream) Close() error     { return nil }

// TestModel_CancelPropagatesToProvider is the T-10a HIGH-1 regression: the
// ctx passed to provider.Stream must be the SAME cancel ctx wired into the
// Model, so a user cancel tears down the HTTP stream — not just the
// TokenStream. Previously streamStartCmd passed context.Background().
func TestModel_CancelPropagatesToProvider(t *testing.T) {
	t.Parallel()
	cp := &captureProvider{gotCtx: make(chan context.Context, 1)}
	m := typeAndModel(t, newFakeModel(cp), "q")
	mm, startCmd := m.Update(keyAction(terminal.KeyEnter, 0))
	if startCmd == nil {
		t.Fatal("submit returned nil startCmd")
	}
	startCmd() // launches consumeStream goroutine; provider captures ctx
	var gotCtx context.Context
	select {
	case gotCtx = <-cp.gotCtx:
	case <-time.After(2 * time.Second):
		t.Fatal("provider.Stream was never called")
	}
	// Trigger cancel through the Model and run the returned Cmd.
	_, cancelCmd := mm.Update(program.CancelCmdMsg{})
	if cancelCmd == nil {
		t.Fatal("cancel returned nil cmd")
	}
	cancelCmd()
	select {
	case <-gotCtx.Done(): // success — provider ctx observed the cancel
	case <-time.After(2 * time.Second):
		t.Fatal("provider ctx not cancelled (HIGH-1: stream not torn down on cancel)")
	}
}

func TestModel_BuildRequest_SystemCacheBreak(t *testing.T) {
	t.Parallel()
	m := New(fake.New(), Options{SystemPrompt: "base", MaxTokens: 100})
	req := m.buildRequest("hi")
	if len(req.System) != 1 || !req.System[0].CacheBreak {
		t.Errorf("System = %+v; want 1 block CacheBreak=true", req.System)
	}
	if len(req.Messages) != 1 || req.Messages[0].Role != llm.RoleUser {
		t.Errorf("Messages = %+v; want 1 user msg", req.Messages)
	}
}

// TestModel_BuildRequest_Thinking is the T-10a HIGH-2 regression: thinking
// config must reach llm.Request (was dropped — domain support unreachable).
func TestModel_BuildRequest_Thinking(t *testing.T) {
	t.Parallel()
	m := New(fake.New(), Options{MaxTokens: 4096, ThinkingMode: llm.ThinkingEnabled, ThinkingBudget: 2048})
	req := m.buildRequest("hi")
	if req.ThinkingMode != llm.ThinkingEnabled || req.ThinkingBudget != 2048 {
		t.Errorf("thinking not wired into request: mode=%v budget=%d", req.ThinkingMode, req.ThinkingBudget)
	}
	// Default Options → thinking disabled (no budget required).
	d := New(fake.New(), Options{MaxTokens: 100}).buildRequest("hi")
	if d.ThinkingMode != llm.ThinkingDisabled {
		t.Errorf("default thinking mode = %v; want Disabled", d.ThinkingMode)
	}
}

// --- helpers ---

func (m *Model) withStripThink(v bool) *Model { next := *m; next.stripThink = v; return &next }

func nodeTexts(m *Model) []string {
	out := make([]string, 0, len(m.scrollback))
	ctx := blockCtx()
	for _, n := range m.scrollback {
		b, err := n.Render(ctx)
		if err != nil || b == nil {
			continue
		}
		out = append(out, gridText(b))
	}
	return out
}

func hasNode(m *Model, substr string) bool {
	for _, txt := range nodeTexts(m) {
		if strings.Contains(txt, substr) {
			return true
		}
	}
	return false
}

func hasThinkingBlock(m *Model) bool {
	for _, n := range m.scrollback {
		if _, ok := n.(block.Thinking); ok {
			return true
		}
	}
	return false
}

func thinkingBlockContains(m *Model, substr string) bool {
	for _, n := range m.scrollback {
		if th, ok := n.(block.Thinking); ok && strings.Contains(th.Content, substr) {
			return true
		}
	}
	return false
}

func messageNodeContains(m *Model, substr string) bool {
	for _, n := range m.scrollback {
		if msg, ok := n.(block.Message); ok && strings.Contains(msg.Text, substr) {
			return true
		}
	}
	return false
}

func blockCtx() block.Context { return block.Context{Cols: 200, Rows: 10} }

func gridText(b buffer.Buffer) string {
	cols, rows := b.Size()
	var sb strings.Builder
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			c := b.Cell(x, y)
			if c.Ch <= 0 {
				continue
			}
			sb.WriteRune(c.Ch)
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}
