// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package llmapp

import (
	"context"
	"errors"

	"github.com/sqlrush/opendbx/internal/app/cli/input"
	"github.com/sqlrush/opendbx/internal/app/cli/keybindings"
	"github.com/sqlrush/opendbx/internal/app/cli/program"
	"github.com/sqlrush/opendbx/internal/app/cli/render/block"
	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
	"github.com/sqlrush/opendbx/internal/app/cli/render/scheduler"
	"github.com/sqlrush/opendbx/internal/app/cli/render/streaming"
	"github.com/sqlrush/opendbx/internal/app/cli/render/style"
	"github.com/sqlrush/opendbx/internal/domain/llm"
)

// ctrlBufSize bounds the control channel (R2.2). Generous so consumeStream
// rarely blocks; the reader-Cmd drains one per Update tick.
const ctrlBufSize = 64

// defaultMaxHistory is the fallback message-history cap when Options
// leaves MaxHistory ≤ 0 (R2 MED-4).
const defaultMaxHistory = 50

// Options configures a chat Model (spec-1.20 D-6).
type Options struct {
	ModelName    string
	SystemPrompt string // single base SystemBlock (multi-block → spec-2.10/2.11)
	MaxTokens    int
	MaxHistory   int // message cap (FIFO); ≤0 → defaultMaxHistory
	StripThink   bool
}

// Model is the spec-1.20 production chat Model (replaces demoapp).
type Model struct {
	provider     llm.Provider
	modelName    string
	systemPrompt string
	maxTokens    int
	maxHistory   int
	stripThink   bool

	buffer string
	cursor int

	history    []llm.Message          // bounded FIFO (R2 MED-4)
	stream     *streaming.TokenStream // in-flight render stream (nil = idle)
	control    chan streamControlMsg  // consumeStream → Update (R2.2)
	cancel     context.CancelFunc     // cancels in-flight stream (Cmd/Cleanup)
	streaming  bool
	sawContent bool // any visible content this turn (R2 H-5 / R2.2 HIGH-2)

	scrollback []block.RenderNode
}

var (
	_ program.Model           = (*Model)(nil)
	_ program.InputModel      = (*Model)(nil)
	_ program.StatusSegmenter = (*Model)(nil)
	_ program.Cleanup         = (*Model)(nil)
)

// New constructs a chat Model bound to a provider.
func New(provider llm.Provider, opts Options) *Model {
	mh := opts.MaxHistory
	if mh <= 0 {
		mh = defaultMaxHistory
	}
	mt := opts.MaxTokens
	if mt <= 0 {
		mt = 4096
	}
	return &Model{
		provider:     provider,
		modelName:    opts.ModelName,
		systemPrompt: opts.SystemPrompt,
		maxTokens:    mt,
		maxHistory:   mh,
		stripThink:   opts.StripThink,
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
	case streamDoneMsg:
		next := *m
		next.stream = nil
		next.control = nil
		next.cancel = nil
		next.streaming = false
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
// allocation: WithCancel / make(chan) / NewTokenStream), and returns the
// streamStartCmd. The user message is appended to scrollback + history.
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
	next.history = appendBounded(m.history, llm.Message{
		Role:    llm.RoleUser,
		Content: []llm.ContentBlock{{Type: llm.BlockText, Text: userText}},
	}, m.maxHistory)
	next.scrollback = appendNode(m.scrollback, block.Message{Text: "> " + userText})

	return &next, streamStartCmd(m.provider, req, ts, ctrl, m.stripThink)
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
		System:    sys,
		Messages:  msgs,
		MaxTokens: m.maxTokens,
	}
}

// streamStartCmd starts the provider stream on the worker pool. On an
// immediate error it classifies via finishFromErr (R2.2 MED — connect-time
// Ctrl+C → FinishCancelled) and does NOT consume a nil stream (R2 HIGH-2).
func streamStartCmd(p llm.Provider, req llm.Request, ts *streaming.TokenStream, ctrl chan streamControlMsg, stripThink bool) scheduler.Cmd {
	return func() scheduler.Msg {
		s, err := p.Stream(context.Background(), req)
		if err != nil {
			fr, mapped := finishFromErr(err)
			ctrl <- streamControlMsg{Finish: fr, Err: mapped}
			close(ctrl)
			_ = ts.Close()
			return readControlMsg{}
		}
		go consumeStream(s, ts, ctrl, stripThink)
		return readControlMsg{}
	}
}

// handleControl applies one streamControlMsg (PURE). sawContent is
// accumulated from VisibleContent (R2.2 HIGH-2 — not from the TokenStream).
func (m *Model) handleControl(msg streamControlMsg) (program.Model, scheduler.Cmd) {
	next := *m
	next.sawContent = m.sawContent || msg.VisibleContent
	if msg.Finish.Terminal() {
		next.scrollback = next.appendFinishNode(msg)
		// Do not re-arm; wait for streamDoneMsg (ctrl close) to clear state.
		return &next, m.armReader()
	}
	// Not terminal: re-arm the reader to pull the next control msg.
	return &next, m.armReader()
}

// armReader returns a Cmd that emits readControlMsg so Update re-pulls.
func (m *Model) armReader() scheduler.Cmd {
	return func() scheduler.Msg { return readControlMsg{} }
}

// appendFinishNode appends a terminal status/marker node for the finish
// reason (R2 H-5 thinking-only handling uses sawContent which is already
// accumulated on next before this is called).
func (m *Model) appendFinishNode(msg streamControlMsg) []block.RenderNode {
	// Specific finish reasons first; STREAM_EMPTY is the last-resort case
	// for a genuinely empty (thinking-only) non-Stop end (R2.2 case-order
	// fix — the !sawContent fallback must not shadow ToolUse/Error/Length).
	switch {
	case msg.Finish == llm.FinishToolUse:
		return appendNode(m.scrollback, block.Message{Text: toolUsePlaceholder(msg.ToolUses)})
	case msg.Finish == llm.FinishError && msg.Err != nil && !errors.Is(msg.Err, context.Canceled):
		return appendNode(m.scrollback, block.Message{Text: "[错误: " + msg.Err.Error() + "]"})
	case msg.Finish == llm.FinishRefusal:
		return appendNode(m.scrollback, block.Message{Text: "[LLM.PROVIDER_REFUSAL: 模型拒绝生成]"})
	case !m.sawContent && msg.Finish != llm.FinishStop:
		// No visible content + non-Stop (thinking-only Length / stop_sequence
		// / pause / cancel) → explicit status so the screen is not silently
		// blank (痛点 1.5). Checked before the Length marker so a
		// thinking-only truncation reports empty, not "[截断]".
		return appendNode(m.scrollback, block.Message{Text: "[LLM.STREAM_EMPTY: 无可见输出]"})
	case msg.Finish == llm.FinishLength:
		// Had visible content but hit the token cap.
		return appendNode(m.scrollback, block.Message{Text: "[截断: 达到 max_tokens]"})
	}
	return m.scrollback
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
		paintBufferAt(g, nb, 0, top)
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
// consumeStream goroutine exits (R2.1 leak defense).
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

func toolUsePlaceholder(tools []llm.ToolUse) string {
	if len(tools) == 0 {
		return "[模型请求工具 — 执行待 spec-1.21]"
	}
	return "[模型请求工具 " + tools[0].Name + " — 执行待 spec-1.21]"
}

// paintBufferAt copies src cells into dst at (xOff, yOff); OOB dropped.
func paintBufferAt(dst *buffer.Grid, src buffer.Buffer, xOff, yOff int) {
	if src == nil {
		return
	}
	dstCols, dstRows := dst.Size()
	srcCols, srcRows := src.Size()
	for sy := 0; sy < srcRows; sy++ {
		dy := yOff + sy
		if dy < 0 || dy >= dstRows {
			continue
		}
		for sx := 0; sx < srcCols; sx++ {
			dx := xOff + sx
			if dx < 0 || dx >= dstCols {
				continue
			}
			dst.SetCell(dx, dy, src.Cell(sx, sy))
		}
	}
}
