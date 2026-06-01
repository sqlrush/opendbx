// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File report_cmd_test.go — spec-1.23 D-4/D-6: /report dispatch + render.

package llmapp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/keybindings"
	"github.com/sqlrush/opendbx/internal/app/cli/program"
	"github.com/sqlrush/opendbx/internal/app/cli/render/block"
	"github.com/sqlrush/opendbx/internal/app/report"
	"github.com/sqlrush/opendbx/internal/domain/llm"
)

func submitAction() program.KeyActionMsg {
	return program.KeyActionMsg{Action: keybindings.ActionSubmit}
}

func lastNode(m *Model) block.RenderNode {
	if len(m.scrollback) == 0 {
		return nil
	}
	return m.scrollback[len(m.scrollback)-1]
}

func TestIsReportCommand(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"/report":  true,
		"/report ": false, // ValueWithoutPrefix is "report " — not exact
		"report":   false, // not slash mode
		"/rep":     false,
		"/reportx": false,
		"why slow": false,
		"":         false,
	}
	for in, want := range cases {
		if got := isReportCommand(in); got != want {
			t.Errorf("isReportCommand(%q) = %v; want %v", in, got, want)
		}
	}
}

// TestModel_ReportCommand_NoDiagnosis — /report with no completed run yields a
// friendly note, clears the buffer, and returns no write Cmd.
func TestModel_ReportCommand_NoDiagnosis(t *testing.T) {
	t.Parallel()
	m := &Model{buffer: "/report"}
	out, cmd := m.handleAction(submitAction())
	nm := out.(*Model)
	if cmd != nil {
		t.Error("no-diagnosis /report must not return a write Cmd")
	}
	if nm.buffer != "" {
		t.Errorf("buffer not cleared: %q", nm.buffer)
	}
	bm, ok := lastNode(nm).(block.Message)
	if !ok || !strings.Contains(bm.Text, "还没有可生成报告") {
		t.Errorf("missing friendly note: %+v", lastNode(nm))
	}
}

// TestModel_ReportCommand_WithSnapshot — /report renders a Markdown node and
// returns a write Cmd; it does NOT call submit() (lastSnapshot preserved).
func TestModel_ReportCommand_WithSnapshot(t *testing.T) {
	t.Parallel()
	snap := &report.RunSnapshot{Prompt: "q", FinalAnswer: "a", FinishStatus: llm.FinishStop}
	m := &Model{buffer: "/report", lastSnapshot: snap}
	out, cmd := m.handleAction(submitAction())
	nm := out.(*Model)
	if cmd == nil {
		t.Error("with-snapshot /report must return a write Cmd")
	}
	if nm.buffer != "" {
		t.Errorf("buffer not cleared: %q", nm.buffer)
	}
	if nm.lastSnapshot != snap {
		t.Error("/report reset lastSnapshot — it must not call submit()")
	}
	if _, ok := lastNode(nm).(block.Markdown); !ok {
		t.Errorf("last node = %T; want block.Markdown", lastNode(nm))
	}
}

// TestModel_ReportCommand_StreamingRejected — /report submitted mid-diagnosis
// is rejected with a visible note; no Cmd, lastSnapshot untouched (spec-1.23
// R-fix; post-impl codex MED-1 / cr MED-1).
func TestModel_ReportCommand_StreamingRejected(t *testing.T) {
	t.Parallel()
	snap := &report.RunSnapshot{Prompt: "q", FinalAnswer: "a", FinishStatus: llm.FinishStop}
	m := &Model{buffer: "/report", streaming: true, lastSnapshot: snap}
	out, cmd := m.handleAction(submitAction())
	nm := out.(*Model)
	if cmd != nil {
		t.Error("mid-stream /report must not return a Cmd")
	}
	if nm.lastSnapshot != snap {
		t.Error("mid-stream /report must not mutate lastSnapshot")
	}
	bm, ok := lastNode(nm).(block.Message)
	if !ok || !strings.Contains(bm.Text, "诊断进行中") {
		t.Errorf("missing streaming-busy note: %+v", lastNode(nm))
	}
}

// TestModel_ReportCommand_CmdWritesFile — end-to-end (spec-1.23 D-7; post-impl
// cr HIGH-1): /report renders a Markdown node AND its write Cmd, when executed,
// persists a file whose content equals the rendered source.
func TestModel_ReportCommand_CmdWritesFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	snap := &report.RunSnapshot{Prompt: "为什么慢", FinalAnswer: "加索引", FinishStatus: llm.FinishStop}
	m := &Model{buffer: "/report", lastSnapshot: snap}
	out, cmd := m.handleAction(submitAction())
	if cmd == nil {
		t.Fatal("expected a write Cmd")
	}
	md, ok := lastNode(out.(*Model)).(block.Markdown)
	if !ok {
		t.Fatalf("last node = %T; want block.Markdown", lastNode(out.(*Model)))
	}

	msg := cmd() // execute the IO Cmd off the pure path
	written, ok := msg.(reportWrittenMsg)
	if !ok {
		t.Fatalf("Cmd returned %T (%+v); want reportWrittenMsg", msg, msg)
	}
	got, err := os.ReadFile(written.path)
	if err != nil {
		t.Fatalf("read written report: %v", err)
	}
	if string(got) != md.Source {
		t.Errorf("file content != rendered source\nfile:\n%s\nnode:\n%s", got, md.Source)
	}
}

func TestModel_ReportWrittenMsg_AppendsPath(t *testing.T) {
	t.Parallel()
	m := &Model{}
	out, _ := m.Update(reportWrittenMsg{path: "/tmp/r.md"})
	bm, ok := lastNode(out.(*Model)).(block.Message)
	if !ok || !strings.Contains(bm.Text, "/tmp/r.md") {
		t.Errorf("written-msg node bad: %+v", lastNode(out.(*Model)))
	}
}

func TestModel_ReportWriteFailedMsg_AppendsError(t *testing.T) {
	t.Parallel()
	m := &Model{}
	out, _ := m.Update(reportWriteFailedMsg{err: report.ErrWriteFailed})
	bm, ok := lastNode(out.(*Model)).(block.Message)
	if !ok || !strings.Contains(bm.Text, "写入文件失败") {
		t.Errorf("write-failed node bad: %+v", lastNode(out.(*Model)))
	}
}
