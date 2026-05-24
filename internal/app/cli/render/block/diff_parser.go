// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File diff_parser.go — spec-1.13 D-6 unified diff text parser.
//
// Per Q7 ★B tolerant policy:
//   - Hunk header malformed → hard error (ErrDiffParseHunkHeader)
//   - Line count mismatch (declared vs actual body) → slog.Warn only
//     (R3 MED-2: NOT a registered error; matches git apply --recount).
//
// Per HIGH-3 fix: `\ No newline at end of file` line NOT added to
// Hunk.Lines AND NOT counted vs declared OldLines/NewLines.
//
// Per ❌-5: git-extras headers (`diff --git`, `index`, file mode) are
// stripped (discarded in spec-1.13; spec-2.x may preserve as metadata).

package block

import (
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"

	"github.com/sqlrush/opendbx/internal/platform/errcode"
)

// hunkHeaderRE matches the `@@ -OldStart,OldLines +NewStart,NewLines @@`
// line. OldLines and NewLines are optional (default 1 when omitted).
var hunkHeaderRE = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// fileHeaderRE matches `--- a/path` or `+++ b/path` lines (with optional
// timestamp suffix).
var fileHeaderRE = regexp.MustCompile(`^(?:---|\+\+\+) (\S+)`)

// ParseUnified parses a unified-diff text into a Diff. Returns the
// extracted FilePath via Diff.FilePath (R3 MED-1 signature fix —
// caller receives FilePath via embedded Diff). Tolerant per Q7 ★B:
// line count mismatch produces slog.Warn, not error.
//
// Errors:
//   - ErrDiffParseHunkHeader: malformed `@@` header (Code/Message/Hint).
//
// Per HIGH-3: `\ No newline at end of file` line is NOT added to
// Hunk.Lines and NOT counted vs declared OldLines/NewLines.
func ParseUnified(text string) (Diff, error) {
	var d Diff
	var current *Hunk
	var bodyOldCount, bodyNewCount int

	lines := strings.Split(text, "\n")
	for i, line := range lines {
		// Git-extras headers — strip (discard per ❌-5).
		if strings.HasPrefix(line, "diff --git ") ||
			strings.HasPrefix(line, "index ") ||
			strings.HasPrefix(line, "similarity index ") ||
			strings.HasPrefix(line, "new file mode ") ||
			strings.HasPrefix(line, "deleted file mode ") ||
			strings.HasPrefix(line, "old mode ") ||
			strings.HasPrefix(line, "new mode ") {
			continue
		}
		// File header: extract FilePath.
		//
		// R3 M1 fix (codex path 2/3): for deleted-file diffs the `+++`
		// side is `/dev/null` (stripPathPrefix → ""), and the `---` side
		// carries the real old path — never let the `/dev/null` empty
		// path overwrite a previously-extracted `--- a/foo.go`.
		//
		// Rule: take `+++` path when it is non-empty (covers both
		// modify and add cases); fall back to `---` path for delete.
		if m := fileHeaderRE.FindStringSubmatch(line); m != nil {
			path := stripPathPrefix(m[1])
			if strings.HasPrefix(line, "+++ ") {
				if path != "" {
					d.FilePath = path
				}
				continue
			}
			// `---` side: only set if we don't already have a path.
			if d.FilePath == "" {
				d.FilePath = path
			}
			continue
		}
		// Hunk header.
		if strings.HasPrefix(line, "@@") {
			if current != nil {
				finalizeHunk(current, bodyOldCount, bodyNewCount)
				d.Hunks = append(d.Hunks, *current)
			}
			h, err := parseHunkHeader(line, i+1)
			if err != nil {
				// errcode-lint:exempt -- spec-1.13 D-6: parseHunkHeader wraps ErrDiffParseHunkHeader (registered errcode RENDER.DIFF_PARSE_HUNK_HEADER) via fmt.Errorf %w. Sentinel pass-through, not a fresh error origin.
				return Diff{}, err
			}
			current = &h
			bodyOldCount, bodyNewCount = 0, 0
			continue
		}
		// Body line — only valid inside a hunk.
		if current == nil {
			continue
		}
		// `\ No newline at end of file` — HIGH-3: discard, not counted.
		if strings.HasPrefix(line, `\ `) {
			continue
		}
		if line == "" && i == len(lines)-1 {
			// Trailing empty line from final \n — skip.
			continue
		}
		marker := classifyMarker(line)
		body := line
		if marker != '?' && len(line) > 0 {
			body = line[1:]
		}
		current.Lines = append(current.Lines, LineEntry{Marker: marker, Text: body})
		switch marker {
		case '+':
			bodyNewCount++
		case '-':
			bodyOldCount++
		case ' ':
			bodyOldCount++
			bodyNewCount++
		}
	}
	if current != nil {
		finalizeHunk(current, bodyOldCount, bodyNewCount)
		d.Hunks = append(d.Hunks, *current)
	}
	return d, nil
}

// parseHunkHeader extracts (OldStart, OldLines, NewStart, NewLines) from
// a `@@ -O,L +N,L @@` line. Returns ErrDiffParseHunkHeader on
// malformed input (R3 MED-2 rule 7 errcode contract).
func parseHunkHeader(line string, lineNum int) (Hunk, error) {
	m := hunkHeaderRE.FindStringSubmatch(line)
	if m == nil {
		return Hunk{}, fmt.Errorf("%w: line %d: %s", ErrDiffParseHunkHeader, lineNum, truncate(line, 80))
	}
	h := Hunk{
		OldStart: atoi(m[1]),
		OldLines: 1,
		NewStart: atoi(m[3]),
		NewLines: 1,
	}
	if m[2] != "" {
		h.OldLines = atoi(m[2])
	}
	if m[4] != "" {
		h.NewLines = atoi(m[4])
	}
	return h, nil
}

// finalizeHunk emits a tolerant warning (Q7 ★B) when the body line
// count drifts from the declared OldLines/NewLines (R3 MED-2: slog.Warn
// NOT errcode). Does not modify the hunk.
func finalizeHunk(h *Hunk, bodyOldCount, bodyNewCount int) {
	if bodyOldCount != h.OldLines || bodyNewCount != h.NewLines {
		slog.Warn(
			"diff.ParseUnified: hunk line count drift",
			"declared_old", h.OldLines, "actual_old", bodyOldCount,
			"declared_new", h.NewLines, "actual_new", bodyNewCount,
		)
	}
}

// stripPathPrefix removes `a/` / `b/` git path prefix from a unified
// diff file header path. Returns input unchanged when no prefix.
func stripPathPrefix(p string) string {
	switch {
	case strings.HasPrefix(p, "a/"):
		return p[2:]
	case strings.HasPrefix(p, "b/"):
		return p[2:]
	case p == "/dev/null":
		return ""
	}
	return p
}

// atoi converts a decimal digit string to int. The input is always a
// hunkHeaderRE capture group ((\d+) — digits only), so strconv.Atoi
// cannot fail; the error discard is safe by regex invariant
// (R2 NIT-2 documented).
func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// ErrDiffParseHunkHeader is the registered errcode sentinel for malformed
// `@@` header per spec-1.13 R3 MED-2 + rule 7 Code/Message/Hint triple
// (spec-0.6 § 2.2.1 registry). Matched via errors.Is in callers that need
// to discriminate.
//
//nolint:gochecknoglobals // spec-0.6 contract: errcode sentinels are package-level.
var ErrDiffParseHunkHeader = errcode.Register(
	"RENDER.DIFF_PARSE_HUNK_HEADER",
	"malformed unified-diff hunk header",
	"verify the `@@ -O,L +N,L @@` syntax of the offending line; opendbx tolerates body-count drift via slog.Warn but rejects malformed headers outright",
)

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
