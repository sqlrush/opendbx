// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package diagnose

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/sqlrush/opendbx/internal/domain/llm"
)

// ============================================================
// Test fixtures: stub provider + recorder emit
// ============================================================

// stubTurn scripts one provider.Stream invocation.
type stubTurn struct {
	chunks       []llm.Chunk // chunks delivered by Next/Chunk in order
	constructErr error       // if non-nil, Stream returns this and never delivers chunks
	blockCtx     bool        // if true, stream blocks until ctx.Done (drives total-timeout / cancel paths)
	streamErr    error       // post-drain Err(); non-ctx errors only — ctx errors are set automatically
}

// stubProv replays scripted turns. Each Stream call consumes one entry.
type stubProv struct {
	turns []stubTurn
	idx   int
}

func (p *stubProv) Name() string { return "stub" }
func (p *stubProv) Stream(ctx context.Context, _ llm.Request) (llm.Stream, error) {
	if p.idx >= len(p.turns) {
		return nil, errors.New("stub provider out of turns (test bug)")
	}
	t := p.turns[p.idx]
	p.idx++
	if t.constructErr != nil {
		return nil, t.constructErr
	}
	return &stubStream{turn: t, ctx: ctx}, nil
}

type stubStream struct {
	turn  stubTurn
	idx   int
	cur   llm.Chunk
	ctx   context.Context
	err   error
	close bool
}

func (s *stubStream) Next() bool {
	if s.turn.blockCtx {
		<-s.ctx.Done()
		s.err = s.ctx.Err()
		return false
	}
	if s.ctx.Err() != nil {
		s.err = s.ctx.Err()
		return false
	}
	if s.idx >= len(s.turn.chunks) {
		if s.turn.streamErr != nil {
			s.err = s.turn.streamErr
		}
		return false
	}
	s.cur = s.turn.chunks[s.idx]
	s.idx++
	return true
}
func (s *stubStream) Chunk() llm.Chunk { return s.cur }
func (s *stubStream) Err() error       { return s.err }
func (s *stubStream) Close() error     { s.close = true; return nil }

// recorder collects every Event so tests can assert the full sequence.
type recorder struct {
	events []Event
	err    error // emit override: if set, emit returns this on Nth call
	at     int   // 1-based index at which err fires; 0 = never
}

func (r *recorder) emit(_ context.Context, e Event) error {
	r.events = append(r.events, e)
	if r.at > 0 && len(r.events) == r.at && r.err != nil {
		return r.err
	}
	return nil
}

// userReq builds a single-turn user message — every test starts this way.
func userReq(text string, tools ...llm.ToolSchema) llm.Request {
	return llm.Request{
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: []llm.ContentBlock{{Type: llm.BlockText, Text: text}}},
		},
		Tools:     tools,
		MaxTokens: 1024,
	}
}

func mustNewLoop(t *testing.T, opt Options) *Loop {
	t.Helper()
	l, err := NewLoop(opt)
	if err != nil {
		t.Fatalf("NewLoop: %v", err)
	}
	return l
}

// finishEvent returns the last EventFinish in r.events (must exist).
func finishEvent(t *testing.T, r *recorder) Event {
	t.Helper()
	for i := len(r.events) - 1; i >= 0; i-- {
		if r.events[i].Kind == EventFinish {
			return r.events[i]
		}
	}
	t.Fatalf("no EventFinish emitted; events=%+v", r.events)
	return Event{}
}

// ============================================================
// NewLoop construction
// ============================================================

func TestNewLoop_RequiresProvider(t *testing.T) {
	t.Parallel()
	_, err := NewLoop(Options{})
	if !errors.Is(err, llm.ErrRequestInvalid) {
		t.Errorf("nil provider → %v; want ErrRequestInvalid", err)
	}
}

func TestNewLoop_AppliesDefaults(t *testing.T) {
	t.Parallel()
	l, err := NewLoop(Options{Provider: &stubProv{}})
	if err != nil {
		t.Fatalf("NewLoop: %v", err)
	}
	if l.maxTurns != DefaultMaxTurns || l.toolTimeout != DefaultToolTimeout ||
		l.totalTimeout != DefaultTotalTimeout || l.reqTimeout != DefaultReqTimeout {
		t.Errorf("defaults not applied: %+v", l)
	}
}

// ============================================================
// Happy paths: single turn with each natural finish reason
// ============================================================

func TestRun_HappyPath_FinishStop(t *testing.T) {
	t.Parallel()
	prov := &stubProv{turns: []stubTurn{
		{chunks: []llm.Chunk{
			{Token: "hello"},
			{FinishReason: llm.FinishStop},
		}},
	}}
	r := &recorder{}
	res, err := mustNewLoop(t, Options{Provider: prov}).
		Run(context.Background(), userReq("hi"), r.emit)
	if err != nil {
		t.Fatalf("Run err: %v", err)
	}
	if res.FinishReason != llm.FinishStop || res.TermCode != "" || res.Turns != 1 {
		t.Errorf("Result = %+v; want FinishStop/empty/1", res)
	}
	if len(res.Messages) != 1 {
		t.Errorf("Messages = %d; want 1 (no-tool keeps caller input only)", len(res.Messages))
	}
	// Event sequence: TurnStart, Text(hello), Finish.
	if len(r.events) != 3 ||
		r.events[0].Kind != EventTurnStart ||
		r.events[1].Kind != EventText || r.events[1].Text != "hello" ||
		r.events[2].Kind != EventFinish {
		t.Errorf("event sequence wrong: %+v", r.events)
	}
}

func TestRun_FinishStopSequence_NaturalTerminate(t *testing.T) {
	t.Parallel()
	prov := &stubProv{turns: []stubTurn{
		{chunks: []llm.Chunk{{FinishReason: llm.FinishStopSequence}}},
	}}
	r := &recorder{}
	res, err := mustNewLoop(t, Options{Provider: prov}).Run(context.Background(), userReq("hi"), r.emit)
	if err != nil {
		t.Errorf("FinishStopSequence should not return error; got %v", err)
	}
	if res.FinishReason != llm.FinishStopSequence || res.TermCode != "" {
		t.Errorf("Result = %+v; want FinishStopSequence/empty", res)
	}
}

func TestRun_FinishLength_NaturalTerminate(t *testing.T) {
	t.Parallel()
	prov := &stubProv{turns: []stubTurn{
		{chunks: []llm.Chunk{{Token: "partial"}, {FinishReason: llm.FinishLength}}},
	}}
	r := &recorder{}
	res, err := mustNewLoop(t, Options{Provider: prov}).Run(context.Background(), userReq("hi"), r.emit)
	if err != nil {
		t.Errorf("FinishLength should not return error; got %v", err)
	}
	if res.FinishReason != llm.FinishLength || res.TermCode != "" {
		t.Errorf("Result = %+v; want FinishLength/empty", res)
	}
}

func TestRun_FinishRefusal_Terminates(t *testing.T) {
	t.Parallel()
	prov := &stubProv{turns: []stubTurn{
		{chunks: []llm.Chunk{{FinishReason: llm.FinishRefusal}}},
	}}
	r := &recorder{}
	_, err := mustNewLoop(t, Options{Provider: prov}).Run(context.Background(), userReq("hi"), r.emit)
	if !errors.Is(err, llm.ErrProviderRefusal) {
		t.Errorf("Run err = %v; want ErrProviderRefusal", err)
	}
	if e := finishEvent(t, r); !errors.Is(e.Err, llm.ErrProviderRefusal) {
		t.Errorf("Finish.Err = %v; want ErrProviderRefusal", e.Err)
	}
}

func TestRun_FinishPause_UnexpectedPauseTermCode(t *testing.T) {
	t.Parallel()
	prov := &stubProv{turns: []stubTurn{
		{chunks: []llm.Chunk{{FinishReason: llm.FinishPause}}},
	}}
	r := &recorder{}
	res, err := mustNewLoop(t, Options{Provider: prov}).Run(context.Background(), userReq("hi"), r.emit)
	if !errors.Is(err, ErrUnexpectedPause) {
		t.Errorf("Run err = %v; want ErrUnexpectedPause", err)
	}
	if res.TermCode != "DIAGNOSE.UNEXPECTED_PAUSE" {
		t.Errorf("TermCode = %q; want DIAGNOSE.UNEXPECTED_PAUSE", res.TermCode)
	}
}

// ============================================================
// Tool round-trip happy path
// ============================================================

func TestRun_ToolRoundTrip_HappyAndPairedCommit(t *testing.T) {
	t.Parallel()
	reg, _ := NewRegistry(ClockTool{Now: func() time.Time {
		return time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	}}, EchoTool{})
	prov := &stubProv{turns: []stubTurn{
		// Turn 1: model asks for clock.
		{chunks: []llm.Chunk{
			{Token: "let me check"},
			{
				FinishReason: llm.FinishToolUse,
				ToolUses:     []llm.ToolUse{{ID: "call_1", Name: "clock"}},
			},
		}},
		// Turn 2: model summarises and stops.
		{chunks: []llm.Chunk{
			{Token: "got it"},
			{FinishReason: llm.FinishStop},
		}},
	}}
	r := &recorder{}
	res, err := mustNewLoop(t, Options{Provider: prov, Registry: reg}).
		Run(context.Background(), userReq("when"), r.emit)
	if err != nil {
		t.Fatalf("Run err: %v", err)
	}
	if res.FinishReason != llm.FinishStop || res.Turns != 2 {
		t.Errorf("Result = %+v; want FinishStop/2", res)
	}
	// Paired commit: caller input + 1 assistant tool_use + 1 user tool_result = 3 msgs.
	if len(res.Messages) != 3 {
		t.Fatalf("Messages = %d; want 3 (user + asst tool_use + user tool_result)", len(res.Messages))
	}
	asst := res.Messages[1]
	if asst.Role != llm.RoleAssistant || asst.Content[len(asst.Content)-1].Type != llm.BlockToolUse {
		t.Errorf("turn 2 message wrong: %+v", asst)
	}
	if got := asst.Content[len(asst.Content)-1].ToolUse.ID; got != "call_1" {
		t.Errorf("assistant ToolUse.ID = %q; want call_1", got)
	}
	user := res.Messages[2]
	if user.Role != llm.RoleUser || user.Content[0].Type != llm.BlockToolResult {
		t.Errorf("turn 3 message wrong: %+v", user)
	}
	if got := user.Content[0].ToolResult.ToolUseID; got != "call_1" {
		t.Errorf("ToolResult.ToolUseID = %q; want call_1", got)
	}
	if got := user.Content[0].ToolResult.Content; got != "2026-05-29T12:00:00Z" {
		t.Errorf("ToolResult.Content = %q; want clock RFC3339", got)
	}
	// Event sequence has TurnStart×2, Text, ToolCall, ToolResult, Text, Finish.
	wantKinds := []EventKind{
		EventTurnStart, EventText, EventToolCall, EventToolResult,
		EventTurnStart, EventText, EventFinish,
	}
	if len(r.events) != len(wantKinds) {
		t.Fatalf("event count = %d; want %d (%+v)", len(r.events), len(wantKinds), r.events)
	}
	for i, k := range wantKinds {
		if r.events[i].Kind != k {
			t.Errorf("event[%d].Kind = %v; want %v", i, r.events[i].Kind, k)
		}
	}
}

// ============================================================
// Tool errors: feedback (回填) vs terminate
// ============================================================

// slowTool sleeps until ctx fires; used to drive per-tool / total timeout paths.
type slowTool struct{ delay time.Duration }

func (slowTool) Name() string { return "slow" }
func (slowTool) Schema() llm.ToolSchema {
	return llm.ToolSchema{Name: "slow", InputSchema: map[string]any{"type": "object"}}
}
func (s slowTool) Execute(ctx context.Context, _ map[string]any) (ToolOutput, error) {
	select {
	case <-time.After(s.delay):
		return ToolOutput{Content: "done"}, nil
	case <-ctx.Done():
		// errcode-lint:exempt -- spec-1.21 D-3 / D-4: ctx errors pass through unchanged; Loop classifies cancel-vs-timeout (D-4).
		return ToolOutput{}, ctx.Err()
	}
}

func TestRun_ToolTimeout_FillbackContinues(t *testing.T) {
	t.Parallel()
	reg, _ := NewRegistry(slowTool{delay: 100 * time.Millisecond})
	prov := &stubProv{turns: []stubTurn{
		{chunks: []llm.Chunk{{
			FinishReason: llm.FinishToolUse,
			ToolUses:     []llm.ToolUse{{ID: "c1", Name: "slow"}},
		}}},
		// After fill-back, model continues and stops.
		{chunks: []llm.Chunk{{FinishReason: llm.FinishStop}}},
	}}
	r := &recorder{}
	res, err := mustNewLoop(t, Options{
		Provider:     prov,
		Registry:     reg,
		ToolTimeout:  10 * time.Millisecond, // fires before slowTool 100ms delay
		TotalTimeout: 5 * time.Second,       // generous → not the cause
		ReqTimeout:   2 * time.Second,
	}).Run(context.Background(), userReq("go"), r.emit)
	if err != nil {
		t.Fatalf("Tool timeout should NOT terminate (回填类); got err=%v", err)
	}
	if res.FinishReason != llm.FinishStop || res.Turns != 2 {
		t.Errorf("Result = %+v; want FinishStop after 2 turns", res)
	}
	// The committed user turn (msgs[2]) carries a ToolResult with IsError=true
	// and the DIAGNOSE.TOOL_TIMEOUT message content.
	user := res.Messages[2]
	tr := user.Content[0].ToolResult
	if !tr.IsError {
		t.Errorf("IsError = false; want true on per-tool timeout fill-back")
	}
	if !strings.Contains(tr.Content, "DIAGNOSE.TOOL_TIMEOUT") {
		t.Errorf("Content = %q; want to contain DIAGNOSE.TOOL_TIMEOUT", tr.Content)
	}
}

func TestRun_ToolReturnsError_FillbackContinues(t *testing.T) {
	t.Parallel()
	// errTool always fails with a non-ctx error → recoverable fill-back.
	reg, _ := NewRegistry(errTool{})
	prov := &stubProv{turns: []stubTurn{
		{chunks: []llm.Chunk{{
			FinishReason: llm.FinishToolUse,
			ToolUses:     []llm.ToolUse{{ID: "c1", Name: "fail"}},
		}}},
		{chunks: []llm.Chunk{{FinishReason: llm.FinishStop}}},
	}}
	r := &recorder{}
	res, err := mustNewLoop(t, Options{Provider: prov, Registry: reg}).
		Run(context.Background(), userReq("go"), r.emit)
	if err != nil {
		t.Errorf("tool Go-error should fill-back, not terminate; got %v", err)
	}
	if res.Turns != 2 {
		t.Errorf("want 2 turns; got %d", res.Turns)
	}
	tr := res.Messages[2].Content[0].ToolResult
	if !tr.IsError || !strings.Contains(tr.Content, "bang") {
		t.Errorf("ToolResult = %+v; want IsError + 'bang'", tr)
	}
}

type errTool struct{}

func (errTool) Name() string { return "fail" }
func (errTool) Schema() llm.ToolSchema {
	return llm.ToolSchema{Name: "fail", InputSchema: map[string]any{"type": "object"}}
}
func (errTool) Execute(context.Context, map[string]any) (ToolOutput, error) {
	return ToolOutput{}, errors.New("bang")
}

// ============================================================
// Unknown tool: terminate before committing assistant turn
// ============================================================

func TestRun_UnknownTool_TerminatesAndDropsAssistantTurn(t *testing.T) {
	t.Parallel()
	reg, _ := NewRegistry(ClockTool{})
	prov := &stubProv{turns: []stubTurn{
		{chunks: []llm.Chunk{
			{Token: "doing"},
			{
				FinishReason: llm.FinishToolUse,
				ToolUses:     []llm.ToolUse{{ID: "c1", Name: "ghost"}},
			},
		}},
	}}
	r := &recorder{}
	res, err := mustNewLoop(t, Options{Provider: prov, Registry: reg}).
		Run(context.Background(), userReq("hi"), r.emit)
	if !errors.Is(err, ErrToolUnknown) {
		t.Errorf("Run err = %v; want ErrToolUnknown", err)
	}
	if res.TermCode != "DIAGNOSE.TOOL_UNKNOWN" {
		t.Errorf("TermCode = %q; want DIAGNOSE.TOOL_UNKNOWN", res.TermCode)
	}
	// Critical invariant: NO assistant turn committed (orphan tool_use
	// would 400 a subsequent provider call — T-2 HIGH-5).
	if len(res.Messages) != 1 {
		t.Errorf("Messages = %d; want 1 (only caller's input — assistant dropped)", len(res.Messages))
	}
}

// ============================================================
// Total timeout: terminal + nothing committed
// ============================================================

func TestRun_TotalTimeout_Terminates(t *testing.T) {
	t.Parallel()
	reg, _ := NewRegistry(slowTool{delay: 5 * time.Second})
	prov := &stubProv{turns: []stubTurn{
		{chunks: []llm.Chunk{{
			FinishReason: llm.FinishToolUse,
			ToolUses:     []llm.ToolUse{{ID: "c1", Name: "slow"}},
		}}},
	}}
	r := &recorder{}
	res, err := mustNewLoop(t, Options{
		Provider:     prov,
		Registry:     reg,
		TotalTimeout: 30 * time.Millisecond,
		ToolTimeout:  10 * time.Second, // generous → total fires first
		ReqTimeout:   10 * time.Second,
	}).Run(context.Background(), userReq("go"), r.emit)
	if !errors.Is(err, ErrTotalTimeout) {
		t.Errorf("Run err = %v; want ErrTotalTimeout", err)
	}
	if res.TermCode != "DIAGNOSE.TOTAL_TIMEOUT" {
		t.Errorf("TermCode = %q; want DIAGNOSE.TOTAL_TIMEOUT", res.TermCode)
	}
	// Nothing committed → only caller input remains.
	if len(res.Messages) != 1 {
		t.Errorf("Messages = %d; want 1 (assistant turn dropped per D-4 step 6)", len(res.Messages))
	}
}

// ============================================================
// Max turns: terminal with DIAGNOSE.MAX_TURNS
// ============================================================

func TestRun_MaxTurnsExceeded_Terminates(t *testing.T) {
	t.Parallel()
	reg, _ := NewRegistry(EchoTool{})
	// Every turn asks for echo again — infinite loop, capped by MaxTurns.
	loopTurn := stubTurn{chunks: []llm.Chunk{{
		FinishReason: llm.FinishToolUse,
		ToolUses:     []llm.ToolUse{{ID: "c", Name: "echo"}},
	}}}
	prov := &stubProv{turns: []stubTurn{loopTurn, loopTurn, loopTurn}}
	r := &recorder{}
	res, err := mustNewLoop(t, Options{
		Provider:     prov,
		Registry:     reg,
		MaxTurns:     2, // forces termination after turn 2's tool path
		TotalTimeout: 5 * time.Second,
	}).Run(context.Background(), userReq("loop"), r.emit)
	if !errors.Is(err, ErrMaxTurns) {
		t.Errorf("Run err = %v; want ErrMaxTurns", err)
	}
	if res.TermCode != "DIAGNOSE.MAX_TURNS" {
		t.Errorf("TermCode = %q; want DIAGNOSE.MAX_TURNS", res.TermCode)
	}
	if res.Turns != 2 {
		t.Errorf("Turns = %d; want 2 (MaxTurns cap)", res.Turns)
	}
}

// ============================================================
// emit backpressure / cancel
// ============================================================

func TestRun_EmitReturnsCtxCanceled_TerminatesCancelled(t *testing.T) {
	t.Parallel()
	prov := &stubProv{turns: []stubTurn{
		{chunks: []llm.Chunk{{Token: "trigger"}, {FinishReason: llm.FinishStop}}},
	}}
	ctx, cancel := context.WithCancel(context.Background())
	emit := func(c context.Context, e Event) error {
		if e.Kind == EventText {
			cancel()
			return c.Err()
		}
		return nil
	}
	_, err := mustNewLoop(t, Options{Provider: prov}).
		Run(ctx, userReq("hi"), emit)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run err = %v; want context.Canceled", err)
	}
}

func TestRun_EmitReturnsGenericError_TerminatesError(t *testing.T) {
	t.Parallel()
	prov := &stubProv{turns: []stubTurn{
		{chunks: []llm.Chunk{{Token: "x"}, {FinishReason: llm.FinishStop}}},
	}}
	bang := errors.New("consumer crash")
	emit := func(_ context.Context, e Event) error {
		if e.Kind == EventText {
			return bang
		}
		return nil
	}
	_, err := mustNewLoop(t, Options{Provider: prov}).
		Run(context.Background(), userReq("hi"), emit)
	if !errors.Is(err, bang) {
		t.Errorf("Run err = %v; want %v (generic emit error)", err, bang)
	}
}

// ============================================================
// Provider construction error: terminal with the upstream code
// ============================================================

func TestRun_ProviderConstructErr_Terminates(t *testing.T) {
	t.Parallel()
	prov := &stubProv{turns: []stubTurn{
		{constructErr: llm.ErrAuthFailed},
	}}
	r := &recorder{}
	_, err := mustNewLoop(t, Options{Provider: prov}).
		Run(context.Background(), userReq("hi"), r.emit)
	if !errors.Is(err, llm.ErrAuthFailed) {
		t.Errorf("Run err = %v; want ErrAuthFailed", err)
	}
}

// ============================================================
// Stream ends without finish (protocol anomaly)
// ============================================================

// TestRun_PerTurnTimeout maps a stream that blocks past reqTimeout (while
// total ctx is healthy) to FinishError + llm.ErrTimeout — the per-turn
// LLM.TIMEOUT path of classifyTerminal.
func TestRun_PerTurnTimeout_MapsToLLMTimeout(t *testing.T) {
	t.Parallel()
	prov := &stubProv{turns: []stubTurn{{blockCtx: true}}}
	_, err := mustNewLoop(t, Options{
		Provider:     prov,
		ReqTimeout:   30 * time.Millisecond,
		TotalTimeout: 5 * time.Second, // generous → per-turn fires first
	}).Run(context.Background(), userReq("hi"), (&recorder{}).emit)
	if !errors.Is(err, llm.ErrTimeout) {
		t.Errorf("Run err = %v; want LLM.TIMEOUT (per-turn deadline)", err)
	}
}

// TestRun_TotalTimeoutViaStream exercises classifyTerminal's
// DeadlineExceeded → TOTAL_TIMEOUT branch on the stream-blocking path
// (the tool-blocking total-timeout case already covered separately).
func TestRun_TotalTimeoutViaStream(t *testing.T) {
	t.Parallel()
	prov := &stubProv{turns: []stubTurn{{blockCtx: true}}}
	res, err := mustNewLoop(t, Options{
		Provider:     prov,
		TotalTimeout: 30 * time.Millisecond,
		ReqTimeout:   5 * time.Second, // generous → total fires first
	}).Run(context.Background(), userReq("hi"), (&recorder{}).emit)
	if !errors.Is(err, ErrTotalTimeout) {
		t.Errorf("Run err = %v; want ErrTotalTimeout", err)
	}
	if res.TermCode != "DIAGNOSE.TOTAL_TIMEOUT" {
		t.Errorf("TermCode = %q; want DIAGNOSE.TOTAL_TIMEOUT", res.TermCode)
	}
}

// TestRun_CallerCtxCancelledMidStream maps a caller ctx cancel during
// stream consumption to FinishCancelled (the classifyTerminal canceled
// path with totalCtx not in deadline state).
func TestRun_CallerCtxCancelledMidStream(t *testing.T) {
	t.Parallel()
	prov := &stubProv{turns: []stubTurn{{blockCtx: true}}}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()
	_, err := mustNewLoop(t, Options{
		Provider:     prov,
		TotalTimeout: 5 * time.Second, // generous
		ReqTimeout:   5 * time.Second,
	}).Run(ctx, userReq("hi"), (&recorder{}).emit)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run err = %v; want context.Canceled", err)
	}
}

// TestRun_NilRegistry_ToolUseMapsToUnknown covers the lookup nil-Registry
// branch: a provider that emits FinishToolUse against a Loop constructed
// without a Registry must surface DIAGNOSE.TOOL_UNKNOWN rather than
// nil-deref.
func TestRun_NilRegistry_ToolUseMapsToUnknown(t *testing.T) {
	t.Parallel()
	prov := &stubProv{turns: []stubTurn{
		{chunks: []llm.Chunk{{
			FinishReason: llm.FinishToolUse,
			ToolUses:     []llm.ToolUse{{ID: "x", Name: "anything"}},
		}}},
	}}
	_, err := mustNewLoop(t, Options{Provider: prov /* no Registry */}).
		Run(context.Background(), userReq("hi"), (&recorder{}).emit)
	if !errors.Is(err, ErrToolUnknown) {
		t.Errorf("nil registry + tool_use → %v; want ErrToolUnknown", err)
	}
}

func TestRun_StreamEndsWithoutFinish_StreamEmpty(t *testing.T) {
	t.Parallel()
	prov := &stubProv{turns: []stubTurn{{chunks: []llm.Chunk{}}}}
	r := &recorder{}
	_, err := mustNewLoop(t, Options{Provider: prov}).
		Run(context.Background(), userReq("hi"), r.emit)
	if !errors.Is(err, llm.ErrStreamEmpty) {
		t.Errorf("Run err = %v; want ErrStreamEmpty", err)
	}
}
