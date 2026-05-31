// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File report_cmd.go — the /report command: render the last completed
// diagnosis snapshot to the TUI (pure) and persist it to disk (in a Cmd, so
// the Update-is-pure contract holds) — spec-1.23 D-4/D-5/D-6.
//
// /report is a minimal inline slash hook: the input package detects slash MODE
// but the slash REGISTRY is spec-2.1, which will later generalize this hook
// into a registered command.

package llmapp

import (
	"time"

	"github.com/sqlrush/opendbx/internal/app/cli/input"
	"github.com/sqlrush/opendbx/internal/app/cli/program"
	"github.com/sqlrush/opendbx/internal/app/cli/render/block"
	"github.com/sqlrush/opendbx/internal/app/cli/render/scheduler"
	"github.com/sqlrush/opendbx/internal/app/report"
)

const (
	reportCommandName     = "report"
	reportNoDiagnosisMsg  = "还没有可生成报告的诊断；先问一个诊断问题，完成后再 /report。"
	reportWrittenPrefix   = "报告已写入: "
	reportWriteFailPrefix = "报告已渲染，但写入文件失败: "
)

// reportWrittenMsg / reportWriteFailedMsg carry the result of the write Cmd.
type reportWrittenMsg struct{ path string }
type reportWriteFailedMsg struct{ err error }

// isReportCommand reports whether buffer is the "/report" slash command.
func isReportCommand(buffer string) bool {
	return input.DeriveMode(buffer) == input.ModeSlash &&
		input.ValueWithoutPrefix(buffer) == reportCommandName
}

// dispatchReport consumes a "/report" submission on the receiver (a *Model copy
// owned by handleAction). It clears the input, renders the report into
// scrollback (pure), and returns a Cmd that writes the file (IO deferred to the
// Cmd). A nil lastSnapshot yields a friendly note and no Cmd.
func (m *Model) dispatchReport() scheduler.Cmd {
	m.buffer = ""
	m.cursor = 0
	if m.lastSnapshot == nil {
		m.scrollback = appendNode(m.scrollback, block.Message{Text: reportNoDiagnosisMsg})
		return nil
	}
	now := time.Now()
	md := report.Generate(*m.lastSnapshot, now)
	m.scrollback = appendNode(m.scrollback, block.NewMarkdown(md))
	return writeReportCmd(md, now)
}

// writeReportCmd performs the (blocking) file write off the pure Update path.
func writeReportCmd(md string, now time.Time) scheduler.Cmd {
	return func() scheduler.Msg {
		path, err := report.WriteReport(md, now)
		if err != nil {
			return reportWriteFailedMsg{err: err}
		}
		return reportWrittenMsg{path: path}
	}
}

func (m *Model) handleReportWritten(msg reportWrittenMsg) (program.Model, scheduler.Cmd) {
	next := *m
	next.scrollback = appendNode(m.scrollback, block.Message{Text: reportWrittenPrefix + msg.path})
	return &next, nil
}

func (m *Model) handleReportWriteFailed(msg reportWriteFailedMsg) (program.Model, scheduler.Cmd) {
	next := *m
	// The report was still rendered to the TUI (R-4: write failure does not
	// block the user from seeing it); append a write-failure note after it.
	next.scrollback = appendNode(m.scrollback, block.Message{Text: reportWriteFailPrefix + msg.err.Error()})
	return &next, nil
}
