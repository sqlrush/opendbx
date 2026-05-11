// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package logger

import (
	"encoding/json"
	"strings"
	"time"
)

// formatEvent renders a log event in the Claude Code text format:
//
//	<ISO ts> [<LEVEL>] <message>\n
//
// CC parity (debug.ts:217-228, verbatim):
//   - timestamp: millisecond precision UTC (matches JS Date.toISOString()
//     output: YYYY-MM-DDTHH:mm:ss.sssZ).
//   - LEVEL: uppercased (VERBOSE / DEBUG / INFO / WARN / ERROR).
//   - message: leading and trailing whitespace trimmed.
//   - trailing newline appended.
//
// Multi-line message handling (spec § 2.2, codex+claude HIGH-1 contract):
//
// CC only JSON-stringifies multi-line messages when hasFormattedOutput is
// true (TUI active). Pre-TUI, multi-line messages pass through verbatim.
// opendbx mirrors this exactly via the package-level atomic flag controlled
// by SetHasFormattedOutput; spec-1.12 tcell-bootstrap flips it on TUI start.
func formatEvent(t time.Time, level Level, message string) string {
	if hasFormattedOutput.Load() && strings.Contains(message, "\n") {
		// jsonStringify equivalent: produces a quoted, escaped, single-line
		// JSON string. json.Marshal on a Go string cannot fail in practice
		// (no invalid UTF-8 surrogate pairs to worry about for typical
		// callers), but if it ever does we fall back to the original
		// message rather than dropping the event.
		if encoded, err := json.Marshal(message); err == nil {
			message = string(encoded)
		}
	}
	var b strings.Builder
	// Pre-size: timestamp (24) + space + [LEVEL] (up to 9) + space + msg + newline
	b.Grow(36 + len(message))
	b.WriteString(formatTimestamp(t))
	b.WriteString(" [")
	b.WriteString(strings.ToUpper(level.String()))
	b.WriteString("] ")
	b.WriteString(strings.TrimSpace(message))
	b.WriteByte('\n')
	return b.String()
}

// formatTimestamp renders t in JS Date.toISOString() format:
//
//	2026-05-10T14:32:01.123Z
//
// Go's stdlib does not provide an exact toISOString() match in time.Format
// constants — RFC3339Nano uses up to 9 fractional digits with trailing zero
// suppression, which diverges from JS's fixed 3-digit millisecond precision.
// We use the explicit reference layout to guarantee CC parity for the
// golden test suite (§ 9 E2E.4).
func formatTimestamp(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}
