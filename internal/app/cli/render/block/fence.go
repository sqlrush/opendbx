// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File fence.go — CommonMark backtick fence parser. spec-1.7 D-3.
//
// Implements **backtick-only** fence detection per CommonMark with
// spec-1.6 R-12 robustness (line-by-line scan within Message.Text, not
// HasPrefix). Tilde `~~~` fence scope removed in spec-1.7 R2 D3 (❌-8;
// spec-1.6 TokenStream only tracks backticks).
//
// CommonMark fence rules implemented:
//   - opening fence: `^[ ]{0,3}\`{3,}[ \t]*([a-zA-Z0-9_+\-.#]*)[ \t]*$`
//     (line-start 0-3 spaces + ≥3 backticks + optional lang label + trailing whitespace)
//   - closing fence: `^[ ]{0,3}\`{N,}[ \t]*$` where N ≥ opening run length
//   - body: lines between opening and closing (exclusive)
//   - 4+ space indent: NOT fence (would be indented code block, CommonMark)
//   - 1-2 backticks: NOT fence (inline code only)
//   - unclosed fence: runs to end of text; Closed=false
//
// Lang label regex `[a-zA-Z0-9_+\-.#]*` (spec-1.7 R2 D6 HIGH-J): includes
// `#` for c#/f#, `.` for .sh; CommonMark also allows other chars but
// MVP regex narrow.

package block

import (
	"strings"
	"unicode"
)

// fenceRange describes one CommonMark backtick fence range within text.
// spec-1.7 R2 D7 polish: full struct with FenceChar / OpenRun / Closed
// / EndLine (EndLine = len(lines)-1 when unclosed).
type fenceRange struct {
	Start int // 0-based line index of opening fence
	End   int // 0-based line index of closing fence (inclusive);
	// for unclosed: len(lines)-1
	Lang      string // optional lang label after opening fence
	FenceChar byte   // '`' (tilde excluded per R2 D3)
	OpenRun   int    // backtick run length of opening (>= 3)
	Closed    bool   // false if fence runs to end-of-text (unclosed)
}

// scanFenceRanges returns ordered fence ranges in text per CommonMark.
// Empty input returns nil. spec-1.7 R-12 robust: handles prose+fence
// bundles (multiple fences inside single Message.Text).
func scanFenceRanges(text string) []fenceRange {
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	var ranges []fenceRange
	i := 0
	for i < len(lines) {
		openRun, lang, ok := parseFenceOpen(lines[i])
		if !ok {
			i++
			continue
		}
		// Found opening fence at line i. Scan for closing fence.
		open := fenceRange{
			Start:     i,
			Lang:      lang,
			FenceChar: '`',
			OpenRun:   openRun,
			Closed:    false,
		}
		j := i + 1
		for j < len(lines) {
			if isFenceClose(lines[j], openRun) {
				open.End = j
				open.Closed = true
				break
			}
			j++
		}
		if !open.Closed {
			// Unclosed: runs to end of text.
			open.End = len(lines) - 1
		}
		ranges = append(ranges, open)
		// Resume scan after closing fence (or after EOF if unclosed).
		if open.Closed {
			i = open.End + 1
		} else {
			i = len(lines)
		}
	}
	return ranges
}

// parseFenceOpen returns (openRun, lang, true) if line matches the
// CommonMark opening fence pattern; otherwise (0, "", false).
func parseFenceOpen(line string) (int, string, bool) {
	// Count leading spaces (0-3 only; tab and 4+ disqualify).
	indent := 0
	for indent < len(line) && line[indent] == ' ' {
		indent++
	}
	if indent > 3 {
		return 0, "", false
	}
	rest := line[indent:]
	// Count backtick run.
	run := 0
	for run < len(rest) && rest[run] == '`' {
		run++
	}
	if run < 3 {
		return 0, "", false
	}
	// Lang label: optional, regex [a-zA-Z0-9_+\-.#]* + optional trailing whitespace.
	tail := strings.TrimLeftFunc(rest[run:], unicode.IsSpace)
	lang := ""
	for k := 0; k < len(tail); k++ {
		c := tail[k]
		if isLangChar(c) {
			lang += string(c)
			continue
		}
		break
	}
	// After lang, only trailing whitespace is allowed (CommonMark).
	after := tail[len(lang):]
	if strings.TrimSpace(after) != "" {
		return 0, "", false
	}
	return run, lang, true
}

// isFenceClose reports whether line matches the closing fence for an
// opening of openRun backticks.
func isFenceClose(line string, openRun int) bool {
	// 0-3 leading spaces.
	indent := 0
	for indent < len(line) && line[indent] == ' ' {
		indent++
	}
	if indent > 3 {
		return false
	}
	rest := line[indent:]
	// Count backtick run.
	run := 0
	for run < len(rest) && rest[run] == '`' {
		run++
	}
	if run < openRun {
		return false
	}
	// After closing run, only trailing whitespace allowed.
	tail := rest[run:]
	return strings.TrimSpace(tail) == ""
}

// isLangChar reports whether c is a valid lang label character per
// spec-1.7 R2 D6 HIGH-J regex [a-zA-Z0-9_+\-.#].
func isLangChar(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z':
		return true
	case c >= 'A' && c <= 'Z':
		return true
	case c >= '0' && c <= '9':
		return true
	case c == '_' || c == '+' || c == '-' || c == '.' || c == '#':
		return true
	}
	return false
}
