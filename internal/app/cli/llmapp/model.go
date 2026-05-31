// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package llmapp

import (
	"context"
	"errors"
	"time"

	"github.com/sqlrush/opendbx/internal/app/cli/input"
	"github.com/sqlrush/opendbx/internal/app/cli/keybindings"
	"github.com/sqlrush/opendbx/internal/app/cli/program"
	"github.com/sqlrush/opendbx/internal/app/cli/render/block"
	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
	"github.com/sqlrush/opendbx/internal/app/cli/render/paint"
	"github.com/sqlrush/opendbx/internal/app/cli/render/scheduler"
	"github.com/sqlrush/opendbx/internal/app/cli/render/streaming"
	"github.com/sqlrush/opendbx/internal/app/cli/render/style"
	"github.com/sqlrush/opendbx/internal/app/diagnose"
	"github.com/sqlrush/opendbx/internal/app/report"
	"github.com/sqlrush/opendbx/internal/domain/llm"
)

// ctrlBufSize bounds the control channel (R2.2). Generous so the
// loopStartCmd emit goroutine rarely blocks; the reader-Cmd drains one
// per Update tick.
const ctrlBufSize = 64

// defaultMaxHistory is the fallback message-history cap when Options
// leaves MaxHistory ≤ 0 (R2 MED-4).
const defaultMaxHistory = 50

// defaultMaxTokens is the fallback response cap when Options.MaxTokens ≤ 0
// (T-10a LOW-1; Anthropic requires max_tokens > 0).
const defaultMaxTokens = 4096

// Options configures a chat Model (spec-1.20 D-6 + spec-1.21 D-6).
type Options struct {
	ModelName    string
	SystemPrompt string // single base SystemBlock (multi-block → spec-2.10/2.11)
	MaxTokens    int
	MaxHistory   int // message cap (FIFO); ≤0 → defaultMaxHistory
	StripThink   bool
	// ThinkingMode / ThinkingBudget wire request-side extended thinking
	// from config into every llm.Request (T-10a HIGH-2 — previously the
	// domain/adapter support was unreachable from production). Budget is
	// validated (≥1024, < MaxTokens) by llm.ValidateRequest when enabled.
	ThinkingMode   llm.ThinkingMode
	ThinkingBudget int

	// Diagnose-loop knobs (spec-1.21 D-6). Registry=nil → single-turn
	// chat mode (FinishToolUse from provider surfaces as DIAGNOSE.
	// TOOL_UNKNOWN); supplying a Registry enables multi-turn function
	// calling. The four timeouts default via diagnose.NewLoop when ≤0.
	Registry     *diagnose.Registry
	MaxTurns     int
	ToolTimeout  time.Duration
	TotalTimeout time.Duration
	ReqTimeout   time.Duration

	// DedupEnabled / DedupWindow wire the spec-1.22 per-Run tool-call dedup
	// cache. DedupEnabled defaults false here (zero value) so a bare
	// Options keeps spec-1.21 behavior; production turns it on via config.
	DedupEnabled bool
	DedupWindow  int
}

// Model is the spec-1.20 production chat Model (replaces demoapp); under
// spec-1.21 D-6 every submit() runs through diagnose.Loop so multi-turn
// tool round-trips are first-class.
type Model struct {
	provider       llm.Provider
	loop           *diagnose.Loop // spec-1.21 D-6 — constructed once in New
	modelName      string
	systemPrompt   string
	maxTokens      int
	maxHistory     int
	stripThink     bool
	thinkingMode   llm.ThinkingMode
	thinkingBudget int

	buffer string
	cursor int

	history     []llm.Message          // bounded FIFO (R2 MED-4)
	stream      *streaming.TokenStream // in-flight render stream (nil = idle)
	control     chan streamControlMsg  // Loop emit → Update (spec-1.21 D-6; legacy R2.2 shape preserved)
	cancel      context.CancelFunc     // cancels in-flight stream (Cmd/Cleanup)
	streaming   bool
	sawContent  bool   // any visible content this turn (R2 H-5 / R2.2 HIGH-2)
	sawThinking bool   // any thinking-channel token this turn (spec-1.20.2 D-5)
	thinkingBuf string // accumulated thinking text when strip_think=false

	// toolUseNames joins ToolResult.ToolUseID → ToolUse.Name within a
	// single submit (spec-1.21 D-6 / spec-1.9b R3 HIGH-3: rendering a
	// ToolResult requires the peer ToolUse.Name; empty → skip render
	// per CC null-return). Reset on every submit.
	toolUseNames map[string]string

	scrollback []block.RenderNode

	// lastSnapshot is the most recent completed (non-cancelled) diagnosis run,
	// captured by the snapshotBuilder and sealed at EventFinish. /report reads
	// it (spec-1.23 D-3/D-4). nil until the first completed run.
	lastSnapshot *report.RunSnapshot
}

var (
	_ program.Model           = (*Model)(nil)
	_ program.InputModel      = (*Model)(nil)
	_ program.StatusSegmenter = (*Model)(nil)
	_ program.Cleanup         = (*Model)(nil)
)

// New constructs a chat Model bound to a provider. The diagnose.Loop is
// built once and reused across submits (spec-1.21 D-6). A nil provider
// panics (programmer error — production wiring always supplies one).
func New(provider llm.Provider, opts Options) *Model {
	mh := opts.MaxHistory
	if mh <= 0 {
		mh = defaultMaxHistory
	}
	mt := opts.MaxTokens
	if mt <= 0 {
		mt = defaultMaxTokens
	}
	loop, err := diagnose.NewLoop(diagnose.Options{
		Provider:     provider,
		Registry:     opts.Registry,
		MaxTurns:     opts.MaxTurns,
		ToolTimeout:  opts.ToolTimeout,
		TotalTimeout: opts.TotalTimeout,
		ReqTimeout:   opts.ReqTimeout,
		DedupEnabled: opts.DedupEnabled,
		DedupWindow:  opts.DedupWindow,
	})
	if err != nil {
		// diagnose.NewLoop only fails on nil Provider — programmer error
		// at wiring time; tests would catch this immediately.
		panic("llmapp.New: " + err.Error())
	}
	return &Model{
		provider:       provider,
		loop:           loop,
		modelName:      opts.ModelName,
		systemPrompt:   opts.SystemPrompt,
		maxTokens:      mt,
		maxHistory:     mh,
		stripThink:     opts.StripThink,
		thinkingMode:   opts.ThinkingMode,
		thinkingBudget: opts.ThinkingBudget,
	}
}

// Init has no startup Cmd.
func (m *Model) Init() scheduler.Cmd { return nil }

// Update is PURE (spec-1.15 / R2 CRIT-1): no IO, no goroutine, no channel
// send. All stream side-effects flow through the returned Cmds.
func (m *Model) Update(msg scheduler.Msg) (program.Model, scheduler.Cmd) {
	switch v := msg.(type) {
	case program.KeyActionMsg:
		return m.handleAction(v)
	case program.CancelCmdMsg:
		return m.handleCancel()
	case readControlMsg:
		// Arm a reader Cmd that pulls one control msg (or a done signal).
		if m.control == nil {
			return m, nil
		}
		ctrl := m.control
		return m, func() scheduler.Msg {
			cm, ok := <-ctrl
			if !ok {
				return streamDoneMsg{}
			}
			return cm
		}
	case streamControlMsg:
		return m.handleControl(v)
	case reportWrittenMsg:
		return m.handleReportWritten(v)
	case reportWriteFailedMsg:
		return m.handleReportWriteFailed(v)
	case streamDoneMsg:
		next := *m
		// T-10a HIGH-3: the TokenStream contract requires a Drain AFTER Close
		// to collect final blocks (Close flushes the last partial into
		// emitted but does not consume it). loopStartCmd closes the
		// TokenStream before closing ctrl, so by now the final blocks are
		// flushed — drain them once more before discarding the stream,
		// else the last tokens
		// are dropped. Update/View share the scheduler goroutine, so this
		// Drain is race-free (same single-owner as the View Drain).
		if next.stream != nil {
			if nodes := next.stream.Drain(); len(nodes) > 0 {
				nodes = filterThinkingOnlyEmpty(nodes, next.sawThinking, next.sawContent)
				if len(nodes) > 0 {
					next.scrollback = appendNodes(next.scrollback, nodes)
				}
			}
		}
		next.stream = nil
		next.control = nil
		next.cancel = nil
		next.streaming = false
		next.sawThinking = false
		next.thinkingBuf = ""
		return &next, nil
	}
	return m, nil
}

// handleAction applies a decoded key Action (input editing + submit).
func (m *Model) handleAction(msg program.KeyActionMsg) (program.Model, scheduler.Cmd) {
	// Ignore input edits while a stream is in flight (single-turn).
	if m.streaming && msg.Action != keybindings.ActionCancel {
		return m, nil
	}
	next := *m
	switch msg.Action {
	case keybindings.ActionInsertRune, keybindings.ActionDeleteBackward, keybindings.ActionDeleteForward:
		next.buffer, next.cursor = input.ResolveMode(m.buffer, m.cursor, msg.Key.Code, msg.Key.Rune)
	case keybindings.ActionMoveLeft, keybindings.ActionMoveRight,
		keybindings.ActionMoveHome, keybindings.ActionMoveEnd:
		next.cursor = input.MoveCursor(m.buffer, m.cursor, program.ActionToMovement(msg.Action))
	case keybindings.ActionSubmit:
		if m.buffer == "" {
			return &next, nil
		}
		// spec-1.23 D-4: /report is intercepted here, BEFORE submit(), so it
		// does not start a new diagnosis or reset lastSnapshot. (The streaming
		// guard at the top of handleAction already blocks any submit mid-run,
		// so /report cannot race a live diagnosis.)
		if isReportCommand(next.buffer) {
			return &next, next.dispatchReport()
		}
		return next.submit()
	case keybindings.ActionCancel:
		return m.handleCancel()
	case keybindings.ActionNone, keybindings.ActionQuit,
		keybindings.ActionHistoryPrev, keybindings.ActionHistoryNext:
		// no-op (Quit → preDispatchSystem; history nav → spec-2.x)
	}
	return &next, nil
}

// submit builds the Request, allocates the stream plumbing (PURE — only
// allocation: WithCancel / make(chan) / NewTokenStream), and returns
// loopStartCmd (spec-1.21 D-6; replaced 1.20's streamStartCmd in T-8).
// The user message is appended to scrollback + history.
func (m *Model) submit() (program.Model, scheduler.Cmd) {
	userText := m.buffer
	req := m.buildRequest(userText)

	ctx, cancel := context.WithCancel(context.Background())
	ts := streaming.NewTokenStream(ctx)
	ctrl := make(chan streamControlMsg, ctrlBufSize)

	next := *m
	next.buffer = ""
	next.cursor = 0
	next.stream = ts
	next.control = ctrl
	next.cancel = cancel
	next.streaming = true
	next.sawContent = false
	next.sawThinking = false
	next.thinkingBuf = ""
	next.history = appendBounded(m.history, llm.Message{
		Role:    llm.RoleUser,
		Content: []llm.ContentBlock{{Type: llm.BlockText, Text: userText}},
	}, m.maxHistory)
	next.scrollback = appendNode(m.scrollback, block.Message{Text: "> " + userText})
	next.toolUseNames = map[string]string{} // reset per submit (spec-1.21 D-6)

	return &next, loopStartCmd(ctx, m.loop, req, userText, ts, ctrl, m.stripThink)
}

// buildRequest assembles the model-agnostic Request from history + the new
// user turn. Single SystemBlock with cache_control (minimal prompt cache).
func (m *Model) buildRequest(userText string) llm.Request {
	var sys []llm.SystemBlock
	if m.systemPrompt != "" {
		sys = []llm.SystemBlock{{Text: m.systemPrompt, CacheBreak: true}}
	}
	msgs := make([]llm.Message, 0, len(m.history)+1)
	msgs = append(msgs, m.history...)
	msgs = append(msgs, llm.Message{
		Role:    llm.RoleUser,
		Content: []llm.ContentBlock{{Type: llm.BlockText, Text: userText}},
	})
	return llm.Request{
		System:         sys,
		Messages:       msgs,
		MaxTokens:      m.maxTokens,
		ThinkingMode:   m.thinkingMode,
		ThinkingBudget: m.thinkingBudget,
	}
}

// handleControl applies one streamControlMsg (PURE; spec-1.20 R2 CRIT-1
// + spec-1.21 D-6 extension). Dispatch order:
//   - ToolUse != nil   → append block.ToolUse (StateRunning) + record
//     name in next.toolUseNames for the matching ToolResult.
//   - ToolResult != nil → append block.ToolResult (joined name lookup;
//     empty name → skip render per CC null-return / spec-1.9b R3 HIGH-3).
//   - Finish terminal  → appendFinishNode (TermCode-aware DIAGNOSE.*).
//   - otherwise        → text variant; accumulate sawContent only.
//
// Every branch re-arms the reader so the next ctrl msg (or done signal)
// is pulled — including the terminal finish, which depends on the close
// signal to trigger streamDoneMsg state cleanup.
func (m *Model) handleControl(msg streamControlMsg) (program.Model, scheduler.Cmd) {
	next := *m
	switch {
	case msg.Thinking:
		next.sawThinking = true
		if msg.ThinkingToken != "" {
			next.thinkingBuf += msg.ThinkingToken
		}
	case msg.ToolUse != nil:
		// Mutate the per-submit map in place — next is already a shallow
		// copy and toolUseNames is owned by this in-flight submit.
		if next.toolUseNames == nil {
			next.toolUseNames = map[string]string{}
		}
		next.toolUseNames[msg.ToolUse.ID] = msg.ToolUse.Name
		tu := block.NewToolUse(msg.ToolUse.ID, msg.ToolUse.Name, msg.ToolUse.Input)
		tu.State = block.StateRunning // caller owns transition per spec-1.9 toolcall.go:62-64
		next.scrollback = appendNode(m.scrollback, tu)
	case msg.ToolResult != nil:
		name := next.toolUseNames[msg.ToolResult.ToolUseID]
		if name == "" {
			// Peer ToolUse name unknown → CC null-return contract: skip
			// render entirely rather than emit a degenerate ToolResult
			// (spec-1.9b R3 HIGH-3).
			break
		}
		// codex T-10a P2-1 absorb: transition the matching block.ToolUse
		// from StateRunning to StateResolved/StateError so the UI does
		// not show the tool as "Running" forever once its result has
		// arrived. Spec-1.9 toolcall.go:62-64 contract: caller owns the
		// transition. spec-1.21 D-6 specifies this exact hand-off:
		// "EventToolResult → 转 Resolved/Error".
		targetState := block.StateResolved
		if msg.ToolResult.IsError {
			targetState = block.StateError
		}
		sb := transitionToolUseState(m.scrollback, msg.ToolResult.ToolUseID, targetState)
		tr := block.NewToolResult(msg.ToolResult.ToolUseID, name, msg.ToolResult.Content, msg.ToolResult.IsError)
		// spec-1.22 D-6: post-set the render-only Cached flag (NewToolResult
		// signature stays unchanged so the ~30 existing call sites don't move).
		tr.Cached = msg.Cached
		next.scrollback = appendNode(sb, tr)
	case msg.Finish.Terminal():
		next.sawContent = m.sawContent || msg.VisibleContent
		next.scrollback = next.appendFinishNode(msg)
		// spec-1.23 D-3: adopt the sealed run snapshot for /report. A cancelled
		// run does NOT overwrite a prior good snapshot ("last completed", not
		// "last attempted" — architect L-2).
		if msg.Snapshot != nil && msg.Finish != llm.FinishCancelled {
			next.lastSnapshot = msg.Snapshot
		}
	default:
		next.sawContent = m.sawContent || msg.VisibleContent
	}
	return &next, m.armReader()
}

// armReader returns a Cmd that emits readControlMsg so Update re-pulls.
func (m *Model) armReader() scheduler.Cmd {
	return func() scheduler.Msg { return readControlMsg{} }
}

// appendFinishNode appends a terminal status/marker node for the finish
// reason. sawContent is already accumulated on next before this is
// called (R2 H-5). spec-1.21 D-6 extension: DIAGNOSE.* TermCodes
// (MAX_TURNS / TOTAL_TIMEOUT / TOOL_UNKNOWN / UNEXPECTED_PAUSE) get
// their own markers so users see the precise terminal cause, not a
// generic "[错误]". 1.21 also retires the FinishToolUse placeholder:
// the Loop now executes the tool internally and emits block.ToolUse /
// block.ToolResult render nodes directly (spec-1.9 / 1.9b consumers).
func (m *Model) appendFinishNode(msg streamControlMsg) []block.RenderNode {
	// DIAGNOSE.* terminal codes first — explicit so a generic FinishError
	// branch does not swallow the precise cause.
	switch msg.TermCode {
	case "DIAGNOSE.MAX_TURNS":
		return appendNode(m.scrollback, block.Message{Text: "[DIAGNOSE.MAX_TURNS: 诊断轮数达上限]"})
	case "DIAGNOSE.TOTAL_TIMEOUT":
		return appendNode(m.scrollback, block.Message{Text: "[DIAGNOSE.TOTAL_TIMEOUT: 诊断总时长超限]"})
	case "DIAGNOSE.TOOL_UNKNOWN":
		return appendNode(m.scrollback, block.Message{Text: "[DIAGNOSE.TOOL_UNKNOWN: 模型请求未注册的工具]"})
	case "DIAGNOSE.UNEXPECTED_PAUSE":
		return appendNode(m.scrollback, block.Message{Text: "[DIAGNOSE.UNEXPECTED_PAUSE: 非预期 pause_turn]"})
	}
	base := m.scrollback
	if m.thinkingBuf != "" {
		base = appendNode(base, block.Thinking{Content: m.thinkingBuf, Collapsed: true})
	}
	if m.sawThinking && !m.sawContent && msg.Finish == llm.FinishStop {
		if m.thinkingBuf != "" {
			return base
		}
		return appendNode(base, block.Message{Text: block.ThinkingOnlyStripMarker()})
	}

	// Specific finish reasons next; STREAM_EMPTY is the last-resort case
	// for a genuinely empty non-Stop end (R2.2 case-order fix — the
	// !sawContent fallback must not shadow Error/Length).
	switch {
	case msg.Finish == llm.FinishCancelled || (msg.Finish == llm.FinishError && errors.Is(msg.Err, context.Canceled)):
		// T-10a MED: a deliberate user cancel gets its own marker so it is
		// not silently blank nor mislabeled STREAM_EMPTY (checked before the
		// !sawContent fallback). msg.Err.Error() is NOT rendered here — the
		// cancel is expected, not an error to surface.
		return appendNode(base, block.Message{Text: "[已取消]"})
	case msg.Finish == llm.FinishError && msg.Err != nil:
		// NB: the SDK's apierror.Error() formats METHOD/URL/STATUS/body only —
		// it does NOT dump request headers, so the API key (X-Api-Key) never
		// leaks here. Do not pass SDK errors to httputil.DumpRequest (T-10a
		// security MED-3).
		return appendNode(base, block.Message{Text: "[错误: " + msg.Err.Error() + "]"})
	case msg.Finish == llm.FinishRefusal:
		return appendNode(base, block.Message{Text: "[LLM.PROVIDER_REFUSAL: 模型拒绝生成]"})
	case !m.sawContent && msg.Finish != llm.FinishStop:
		// No visible content + non-Stop (thinking-only Length / stop_sequence
		// / pause / cancel) → explicit status so the screen is not silently
		// blank (痛点 1.5). Checked before the Length marker so a
		// thinking-only truncation reports empty, not "[截断]".
		return appendNode(base, block.Message{Text: "[LLM.STREAM_EMPTY: 无可见输出]"})
	case msg.Finish == llm.FinishLength:
		// Had visible content but hit the token cap.
		return appendNode(base, block.Message{Text: "[截断: 达到 max_tokens]"})
	}
	return base
}

// handleCancel cancels an in-flight stream via a Cmd (side effect in Cmd,
// not Update — R2 CRIT-1). No-op when idle.
func (m *Model) handleCancel() (program.Model, scheduler.Cmd) {
	if m.cancel == nil {
		next := *m
		next.buffer = ""
		next.cursor = 0
		return &next, nil
	}
	cancel := m.cancel
	return m, func() scheduler.Msg {
		cancel()
		return nil
	}
}

// View renders scrollback + the in-flight stream. Per spec-1.6 main-loop
// bridge (Q11 Drain-in-View): Drain the active stream into scrollback each
// frame (View runs on the scheduler goroutine, single-owner of m).
func (m *Model) View(cols, rows int) buffer.Buffer {
	if m.stream != nil {
		if nodes := m.stream.Drain(); len(nodes) > 0 {
			nodes = filterThinkingOnlyEmpty(nodes, m.sawThinking, m.sawContent)
			m.scrollback = append(m.scrollback, nodes...)
		}
	}
	g, err := buffer.NewGrid(cols, rows)
	if err != nil {
		return nil
	}
	// Paint the most recent nodes bottom-up.
	ctx := block.Context{Cols: cols, Rows: rows}
	y := rows - 1
	for i := len(m.scrollback) - 1; i >= 0 && y >= 0; i-- {
		nb, rerr := m.scrollback[i].Render(ctx)
		if rerr != nil || nb == nil {
			continue
		}
		_, nbRows := nb.Size()
		top := y - nbRows + 1
		paint.BlitAt(g, nb, 0, top)
		y = top - 1
	}
	return g
}

// InputState exposes buffer + cursor to program.paintInputRow.
func (m *Model) InputState() program.InputState {
	return program.InputState{Buffer: m.buffer, Cursor: m.cursor}
}

// StatusSegments shows the model name + a streaming indicator.
func (m *Model) StatusSegments() []program.StatusSegment {
	name := m.modelName
	if name == "" {
		name = m.provider.Name()
	}
	segs := []program.StatusSegment{{Text: name}}
	if m.streaming {
		segs = append(segs, program.StatusSegment{Text: "●", Style: style.Style{Bold: true}})
	}
	return segs
}

// Cleanup cancels any in-flight stream on program shutdown so the
// loopStartCmd emit goroutine exits (R2.1 leak defense).
func (m *Model) Cleanup() scheduler.Cmd {
	if m.cancel == nil {
		return nil
	}
	cancel := m.cancel
	return func() scheduler.Msg {
		cancel()
		return nil
	}
}

// --- helpers ---

func appendBounded(history []llm.Message, msg llm.Message, max int) []llm.Message {
	next := make([]llm.Message, 0, len(history)+1)
	next = append(next, history...)
	next = append(next, msg)
	if len(next) > max {
		next = next[len(next)-max:]
	}
	return next
}

func appendNode(sb []block.RenderNode, n block.RenderNode) []block.RenderNode {
	next := make([]block.RenderNode, 0, len(sb)+1)
	next = append(next, sb...)
	next = append(next, n)
	return next
}

// appendNodes returns a new slice with nodes appended (immutable; T-10a
// HIGH-3 final-drain path).
func appendNodes(sb []block.RenderNode, nodes []block.RenderNode) []block.RenderNode {
	next := make([]block.RenderNode, 0, len(sb)+len(nodes))
	next = append(next, sb...)
	next = append(next, nodes...)
	return next
}

func filterThinkingOnlyEmpty(nodes []block.RenderNode, sawThinking, sawContent bool) []block.RenderNode {
	if !sawThinking || sawContent || len(nodes) == 0 {
		return nodes
	}
	out := make([]block.RenderNode, 0, len(nodes))
	for _, n := range nodes {
		msg, ok := n.(block.Message)
		if ok && msg.Empty {
			continue
		}
		out = append(out, n)
	}
	return out
}

// transitionToolUseState returns a new scrollback slice with the most
// recent block.ToolUse matching id rewritten to the given state. Other
// entries are preserved by identity (interface values are copied; the
// underlying ToolUse value is replaced wholesale because block.ToolUse
// is a value receiver type). If no matching ToolUse exists the original
// slice is returned unmodified — this is the "Loop emitted ToolResult
// without a peer ToolUse" path, which handleControl already guards
// against via the name lookup (spec-1.9b R3 HIGH-3 null-return).
//
// Walks from the tail because within a single submit IDs are unique
// per tool dispatch; the most recent matching ToolUse is the correct
// peer for a result that just arrived (codex T-10a P2-1).
func transitionToolUseState(sb []block.RenderNode, id string, state block.ToolUseState) []block.RenderNode {
	for i := len(sb) - 1; i >= 0; i-- {
		tu, ok := sb[i].(block.ToolUse)
		if !ok || tu.ID != id {
			continue
		}
		out := make([]block.RenderNode, len(sb))
		copy(out, sb)
		tu.State = state
		out[i] = tu
		return out
	}
	return sb
}

// ScrollbackTypesForTest returns the concrete render-node type names of
// every scrollback entry. It exists solely so the spec-1.21 D-8
// integration smoke (tests/integration/uitest/diagnoseloop) can assert
// the Loop → block dispatch produced block.ToolUse / block.ToolResult
// nodes rather than retired placeholders, without poking at unexported
// fields via reflection. Production code MUST NOT call this.
func (m *Model) ScrollbackTypesForTest() []string {
	out := make([]string, len(m.scrollback))
	for i, n := range m.scrollback {
		out[i] = nodeTypeName(n)
	}
	return out
}

// nodeTypeName renders the runtime type name without pulling in fmt
// just for one Sprintf — keeps the dependency surface minimal.
func nodeTypeName(n block.RenderNode) string {
	switch n.(type) {
	case block.Message:
		return "block.Message"
	case block.ToolUse:
		return "block.ToolUse"
	case block.ToolResult:
		return "block.ToolResult"
	case block.Thinking:
		return "block.Thinking"
	}
	return "block.unknown"
}
