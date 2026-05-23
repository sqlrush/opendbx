// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package adapter

import (
	"strings"
	"sync"
	"testing"
)

func TestRegistry_RegisterLookup(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.Register("x", Generic{})
	if got := r.Lookup("x"); got == nil {
		t.Errorf("after Register: Lookup should return registered renderer")
	}
	if got := r.Lookup("missing"); got != nil {
		t.Errorf("unregistered Lookup should return nil, got %v", got)
	}
}

func TestRegistry_ZeroValueUsable(t *testing.T) {
	t.Parallel()
	var r Registry
	r.Register("x", Generic{})
	if got := r.Lookup("x"); got == nil {
		t.Errorf("zero-value Registry: Lookup should return registered renderer")
	}
}

func TestRegistry_ConcurrentWrites(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); r.Register("name", Bash{}) }()
		go func() { defer wg.Done(); _ = r.Lookup("name") }()
	}
	wg.Wait()
}

func TestBash_RenderHeader_Truncate(t *testing.T) {
	t.Parallel()
	b := Bash{}
	long := strings.Repeat("x", 200)
	out, _ := b.RenderHeader(map[string]any{"command": long}, Context{})
	// 160 chars truncated, with '…' rune (3 bytes UTF-8) appended; total ≤ 162 bytes.
	if len(out) > 162 {
		t.Errorf("non-verbose truncate failed: len=%d, want ≤162", len(out))
	}
	if !strings.HasSuffix(out, "…") {
		t.Errorf("non-verbose truncate should end with '…': %q", out)
	}
	outV, _ := b.RenderHeader(map[string]any{"command": long}, Context{Verbose: true})
	if len(outV) != 200 {
		t.Errorf("verbose should not truncate: len=%d", len(outV))
	}
}

func TestBash_RenderHeader_NoCommand(t *testing.T) {
	t.Parallel()
	b := Bash{}
	out, _ := b.RenderHeader(nil, Context{})
	if out != "(no command)" {
		t.Errorf("nil input: want '(no command)', got %q", out)
	}
}

func TestBash_RenderHeader_NonStringCommand(t *testing.T) {
	t.Parallel()
	b := Bash{}
	out, _ := b.RenderHeader(map[string]any{"command": 42}, Context{})
	if out != "(no command)" {
		t.Errorf("non-string command: want '(no command)', got %q", out)
	}
}

func TestBash_RenderProgress(t *testing.T) {
	t.Parallel()
	b := Bash{}
	out, _ := b.RenderProgress(nil, Context{})
	if out != "Running…" {
		t.Errorf("empty progress: got %q", out)
	}
	out2, _ := b.RenderProgress([]ProgressMessage{{ElapsedSeconds: 1, TotalLines: 5, TotalBytes: 100, TimeoutMs: 1000}}, Context{})
	for _, sub := range []string{"1s", "5 lines", "100B", "timeout 1000ms"} {
		if !strings.Contains(out2, sub) {
			t.Errorf("progress missing %q: %q", sub, out2)
		}
	}
}

func TestBash_RenderQueued(t *testing.T) {
	t.Parallel()
	b := Bash{}
	out, _ := b.RenderQueued()
	if out != "Waiting…" {
		t.Errorf("Queued: got %q", out)
	}
}

func TestRead_RenderHeader_Default(t *testing.T) {
	t.Parallel()
	r := Read{}
	out, _ := r.RenderHeader(map[string]any{"path": "main.go"}, Context{})
	if out != "main.go" {
		t.Errorf("default: got %q", out)
	}
}

func TestRead_RenderHeader_VerboseLines(t *testing.T) {
	t.Parallel()
	r := Read{}
	out, _ := r.RenderHeader(map[string]any{"path": "x.go", "offset": 10, "limit": 50}, Context{Verbose: true})
	if !strings.Contains(out, "lines 10-59") {
		t.Errorf("verbose lines: got %q", out)
	}
}

func TestRead_RenderHeader_OffsetOnly(t *testing.T) {
	t.Parallel()
	r := Read{}
	out, _ := r.RenderHeader(map[string]any{"path": "x.go", "offset": 100}, Context{Verbose: true})
	if !strings.Contains(out, "from line 100") {
		t.Errorf("offset only: got %q", out)
	}
}

func TestRead_RenderHeader_LimitOnly(t *testing.T) {
	t.Parallel()
	r := Read{}
	out, _ := r.RenderHeader(map[string]any{"path": "x.go", "limit": 20}, Context{Verbose: true})
	if !strings.Contains(out, "first 20 lines") {
		t.Errorf("limit only: got %q", out)
	}
}

func TestRead_RenderHeader_PagesPDF(t *testing.T) {
	t.Parallel()
	r := Read{}
	out, _ := r.RenderHeader(map[string]any{"path": "x.pdf", "pages": "1-3"}, Context{})
	if out != "x.pdf · pages 1-3" {
		t.Errorf("pages: got %q", out)
	}
}

func TestRead_RenderHeader_NoPath(t *testing.T) {
	t.Parallel()
	r := Read{}
	out, _ := r.RenderHeader(nil, Context{})
	if out != "(no path)" {
		t.Errorf("nil path: got %q", out)
	}
}

func TestRead_ParseStringOffsetLimit(t *testing.T) {
	t.Parallel()
	r := Read{}
	// LLM might send numbers as strings.
	out, _ := r.RenderHeader(map[string]any{"path": "x.go", "offset": "5", "limit": "10"}, Context{Verbose: true})
	if !strings.Contains(out, "lines 5-14") {
		t.Errorf("string offset/limit: got %q", out)
	}
}

func TestGeneric_RenderHeader_Compact(t *testing.T) {
	t.Parallel()
	g := Generic{}
	out, _ := g.RenderHeader(map[string]any{"_name": "T", "a": "1", "b": "2"}, Context{Cols: 80})
	if !strings.Contains(out, "T(") {
		t.Errorf("missing name: %q", out)
	}
	if !strings.Contains(out, "a=1") || !strings.Contains(out, "b=2") {
		t.Errorf("missing args: %q", out)
	}
}

func TestGeneric_NoName(t *testing.T) {
	t.Parallel()
	g := Generic{}
	out, _ := g.RenderHeader(map[string]any{"a": "1"}, Context{Cols: 80})
	if !strings.HasPrefix(out, "(unnamed)") {
		t.Errorf("no name: %q", out)
	}
}

func TestGeneric_TruncateBudget(t *testing.T) {
	t.Parallel()
	g := Generic{}
	out, _ := g.RenderHeader(map[string]any{"_name": "T", "k": strings.Repeat("v", 200)}, Context{Cols: 40})
	if !strings.HasSuffix(out, "…)") && !strings.HasSuffix(out, "…") {
		t.Errorf("budget truncate: got %q", out)
	}
}

func TestGeneric_SkipsSyntheticKeys(t *testing.T) {
	t.Parallel()
	g := Generic{}
	out, _ := g.RenderHeader(map[string]any{"_name": "T", "_meta": "x", "a": "1"}, Context{Cols: 80})
	if strings.Contains(out, "_meta") {
		t.Errorf("should skip _meta synthetic key: %q", out)
	}
}

// === spec-1.9b ToolResult-side adapter tests ===

func TestBash_RenderResult_Compact(t *testing.T) {
	t.Parallel()
	b := Bash{}
	out, _ := b.RenderResult("line1\nline2\nline3", Context{})
	lines := strings.Split(out, "\n")
	if len(lines) > 2 {
		t.Errorf("Bash RenderResult non-verbose: want ≤2 lines, got %d: %q", len(lines), out)
	}
}

func TestBash_RenderResult_Verbose(t *testing.T) {
	t.Parallel()
	b := Bash{}
	out, _ := b.RenderResult("line1\nline2\nline3", Context{Verbose: true})
	if !strings.Contains(out, "line3") {
		t.Errorf("Bash verbose should include all lines: %q", out)
	}
}

func TestBash_RenderResult_Empty(t *testing.T) {
	t.Parallel()
	b := Bash{}
	out, _ := b.RenderResult("", Context{})
	if out != "(empty)" {
		t.Errorf("empty content: got %q", out)
	}
}

func TestBash_RenderResult_StructuredStdoutStderr(t *testing.T) {
	t.Parallel()
	b := Bash{}
	out, _ := b.RenderResult(map[string]any{
		"stdout": "ok",
		"stderr": "warn",
	}, Context{Verbose: true})
	if !strings.Contains(out, "ok") || !strings.Contains(out, "warn") {
		t.Errorf("structured stdout/stderr missing: %q", out)
	}
}

func TestBash_RenderResult_StructuredNoOutputExpected(t *testing.T) {
	t.Parallel()
	b := Bash{}
	out, _ := b.RenderResult(map[string]any{"noOutputExpected": true}, Context{})
	if out != "Done" {
		t.Errorf("noOutputExpected: got %q", out)
	}
}

func TestBash_RenderResult_StructuredImage(t *testing.T) {
	t.Parallel()
	b := Bash{}
	out, _ := b.RenderResult(map[string]any{"isImage": true, "stdout": "..."}, Context{})
	if out != "[Image data detected and sent to Claude]" {
		t.Errorf("image summary: got %q", out)
	}
}

func TestBash_RenderResult_StructuredBackground(t *testing.T) {
	t.Parallel()
	b := Bash{}
	out, _ := b.RenderResult(map[string]any{"backgroundTaskId": "bg1"}, Context{})
	if !strings.Contains(out, "Running in the background") {
		t.Errorf("background summary: got %q", out)
	}
}

func TestBash_RenderResult_StructuredReturnCodeInterpretation(t *testing.T) {
	t.Parallel()
	b := Bash{}
	out, _ := b.RenderResult(map[string]any{"returnCodeInterpretation": "No matches found"}, Context{})
	if out != "No matches found" {
		t.Errorf("returnCodeInterpretation: got %q", out)
	}
}

func TestBash_RenderResult_StructuredNoOutput(t *testing.T) {
	t.Parallel()
	b := Bash{}
	out, _ := b.RenderResult(map[string]any{}, Context{})
	if out != "(No output)" {
		t.Errorf("empty structured output: got %q", out)
	}
}

func TestBash_RenderResult_MapStringString(t *testing.T) {
	t.Parallel()
	b := Bash{}
	out, _ := b.RenderResult(map[string]string{"stdout": "ok"}, Context{})
	if out != "ok" {
		t.Errorf("map[string]string stdout: got %q", out)
	}
}

func TestBash_RenderRejected_Empty_DefersToCallerFallback(t *testing.T) {
	t.Parallel()
	b := Bash{}
	out, _ := b.RenderRejected("", Context{})
	if out != "" {
		t.Errorf("Bash rejected empty should return \"\" to defer to block fallback, got %q", out)
	}
}

func TestBash_RenderRejected_WithContent(t *testing.T) {
	t.Parallel()
	b := Bash{}
	out, _ := b.RenderRejected("user said no", Context{})
	if out != "user said no" {
		t.Errorf("got %q", out)
	}
}

func TestBash_RenderErrorResult(t *testing.T) {
	t.Parallel()
	b := Bash{}
	out, _ := b.RenderErrorResult("exit 1\nsome error", Context{})
	if !strings.Contains(out, "exit 1") {
		t.Errorf("got %q", out)
	}
}

func TestBash_RenderErrorResult_Empty(t *testing.T) {
	t.Parallel()
	b := Bash{}
	out, _ := b.RenderErrorResult(nil, Context{})
	if out != "Tool execution failed" {
		t.Errorf("got %q", out)
	}
}

func TestBash_RenderErrorResult_Verbose(t *testing.T) {
	t.Parallel()
	b := Bash{}
	multi := "line1\nline2\nline3"
	out, _ := b.RenderErrorResult(multi, Context{Verbose: true})
	if !strings.Contains(out, "line3") {
		t.Errorf("verbose should include all lines: %q", out)
	}
}

func TestFormatFallbackToolUseError_InputValidation(t *testing.T) {
	t.Parallel()
	out := FormatFallbackToolUseError("InputValidationError: bad args", Context{})
	if out != "Invalid tool parameters" {
		t.Errorf("input validation compact: got %q", out)
	}
}

func TestFormatFallbackToolUseError_ToolUseErrorTag(t *testing.T) {
	t.Parallel()
	out := FormatFallbackToolUseError("prefix <tool_use_error><error>boom</error></tool_use_error> suffix", Context{})
	if out != "Error: boom" {
		t.Errorf("tool_use_error extraction: got %q", out)
	}
}

func TestFormatFallbackToolUseError_RemovesSandboxViolations(t *testing.T) {
	t.Parallel()
	out := FormatFallbackToolUseError("Error: denied\n<sandbox_violations>secret</sandbox_violations>", Context{})
	if strings.Contains(out, "secret") || !strings.Contains(out, "denied") {
		t.Errorf("sandbox tag cleanup: got %q", out)
	}
}

func TestFormatFallbackToolUseError_CompactTenLines(t *testing.T) {
	t.Parallel()
	out := FormatFallbackToolUseError("l1\nl2\nl3\nl4\nl5\nl6\nl7\nl8\nl9\nl10\nl11", Context{})
	if got := strings.Count(out, "\n") + 1; got != 10 {
		t.Errorf("compact line count: got %d lines in %q", got, out)
	}
	if strings.Contains(out, "l11") {
		t.Errorf("compact should omit line 11: %q", out)
	}
}

func TestRead_RenderResult_LineCount(t *testing.T) {
	t.Parallel()
	r := Read{}
	out, _ := r.RenderResult("a\nb\nc", Context{})
	if !strings.Contains(out, "Read 3 lines") {
		t.Errorf("got %q", out)
	}
}

func TestRead_RenderResult_TrailingNewline(t *testing.T) {
	t.Parallel()
	r := Read{}
	out, _ := r.RenderResult("a\nb\nc\n", Context{})
	if !strings.Contains(out, "Read 3 lines") {
		t.Errorf("trailing newline shouldn't inflate: %q", out)
	}
}

func TestRead_RenderResult_Empty(t *testing.T) {
	t.Parallel()
	r := Read{}
	out, _ := r.RenderResult("", Context{})
	if out != "(empty file)" {
		t.Errorf("got %q", out)
	}
}

func TestRead_RenderResult_Verbose(t *testing.T) {
	t.Parallel()
	r := Read{}
	out, _ := r.RenderResult("a\nb\nc", Context{Verbose: true})
	if out != "a\nb\nc" {
		t.Errorf("verbose raw: got %q", out)
	}
}

func TestRead_RenderResult_StructuredText(t *testing.T) {
	t.Parallel()
	r := Read{}
	out, _ := r.RenderResult(map[string]any{"type": "text", "file": map[string]any{"numLines": 7}}, Context{})
	if out != "Read 7 lines" {
		t.Errorf("structured text: got %q", out)
	}
}

func TestRead_RenderResult_StructuredFileUnchanged(t *testing.T) {
	t.Parallel()
	r := Read{}
	out, _ := r.RenderResult(map[string]any{"type": "file_unchanged"}, Context{})
	if out != "Unchanged since last read" {
		t.Errorf("file_unchanged: got %q", out)
	}
}

func TestRead_RenderResult_StructuredImage(t *testing.T) {
	t.Parallel()
	r := Read{}
	out, _ := r.RenderResult(map[string]any{"type": "image", "file": map[string]any{"originalSize": 1536}}, Context{})
	if out != "Read image (1.5KB)" {
		t.Errorf("image summary: got %q", out)
	}
}

func TestRead_RenderResult_StructuredNotebookEmpty(t *testing.T) {
	t.Parallel()
	r := Read{}
	out, _ := r.RenderResult(map[string]any{"type": "notebook", "file": map[string]any{"cells": []any{}}}, Context{})
	if out != "No cells found in notebook" {
		t.Errorf("empty notebook: got %q", out)
	}
}

func TestRead_RenderResult_StructuredNotebookCells(t *testing.T) {
	t.Parallel()
	r := Read{}
	out, _ := r.RenderResult(map[string]any{"type": "notebook", "file": map[string]any{"cells": []map[string]any{{"cell_type": "code"}}}}, Context{})
	if out != "Read 1 cells" {
		t.Errorf("notebook cells: got %q", out)
	}
}

func TestRead_RenderResult_StructuredPDF(t *testing.T) {
	t.Parallel()
	r := Read{}
	out, _ := r.RenderResult(map[string]any{"type": "pdf", "file": map[string]any{"originalSize": 1048576}}, Context{})
	if out != "Read PDF (1MB)" {
		t.Errorf("pdf summary: got %q", out)
	}
}

func TestRead_RenderResult_StructuredParts(t *testing.T) {
	t.Parallel()
	r := Read{}
	out, _ := r.RenderResult(map[string]any{"type": "parts", "file": map[string]any{"count": "1", "originalSize": 1073741824}}, Context{})
	if out != "Read 1 page (1GB)" {
		t.Errorf("parts summary: got %q", out)
	}
}

func TestRead_RenderResult_StructuredTextSingular(t *testing.T) {
	t.Parallel()
	r := Read{}
	out, _ := r.RenderResult(map[string]any{"type": "text", "file": map[string]any{"numLines": float64(1)}}, Context{})
	if out != "Read 1 line" {
		t.Errorf("structured text singular: got %q", out)
	}
}

func TestRead_RenderResult_StructuredSmallImage(t *testing.T) {
	t.Parallel()
	r := Read{}
	out, _ := r.RenderResult(map[string]any{"type": "image", "file": map[string]any{"originalSize": 42}}, Context{})
	if out != "Read image (42 bytes)" {
		t.Errorf("small image summary: got %q", out)
	}
}

func TestRead_RenderResult_UnknownStructuredFallsBack(t *testing.T) {
	t.Parallel()
	r := Read{}
	out, _ := r.RenderResult(map[string]any{"type": "unknown", "file": map[string]any{"x": 1}}, Context{})
	if !strings.Contains(out, "unknown") {
		t.Errorf("unknown structured fallback: got %q", out)
	}
}

func TestRead_RenderErrorResult_NotFound(t *testing.T) {
	t.Parallel()
	r := Read{}
	out, _ := r.RenderErrorResult("File does not exist. Note: your current working directory is /tmp.", Context{})
	if out != "File not found" {
		t.Errorf("cwd-note file-not-found compact: got %q", out)
	}
}

func TestRead_RenderErrorResult_Empty(t *testing.T) {
	t.Parallel()
	r := Read{}
	out, _ := r.RenderErrorResult("", Context{})
	if out != "Error: " {
		t.Errorf("got %q", out)
	}
}

func TestRead_RenderErrorResult_ToolUseErrorTag(t *testing.T) {
	t.Parallel()
	r := Read{}
	out, _ := r.RenderErrorResult("<tool_use_error>permission denied</tool_use_error>", Context{})
	if out != "Error reading file" {
		t.Errorf("tool_use_error compact: got %q", out)
	}
}

func TestRead_RenderErrorResult_GenericPrefix(t *testing.T) {
	t.Parallel()
	r := Read{}
	out, _ := r.RenderErrorResult("something broke", Context{})
	if !strings.HasPrefix(out, "Error: ") {
		t.Errorf("expected 'Error: ' prefix: %q", out)
	}
}

func TestGeneric_RenderResult_Empty(t *testing.T) {
	t.Parallel()
	g := Generic{}
	out, _ := g.RenderResult("", Context{})
	if out != "(result)" {
		t.Errorf("got %q", out)
	}
}

func TestGeneric_RenderResult_WithContent(t *testing.T) {
	t.Parallel()
	g := Generic{}
	out, _ := g.RenderResult("done", Context{})
	if out != "done" {
		t.Errorf("got %q", out)
	}
}

func TestContentToString_Types(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   any
		want string
	}{
		{nil, ""},
		{"x", "x"},
		{[]byte("y"), "y"},
		{42, "42"},
	}
	for _, c := range cases {
		got := contentToString(c.in)
		if got != c.want {
			t.Errorf("contentToString(%v): got %q, want %q", c.in, got, c.want)
		}
	}
}

func TestStringFromMap_BytesAndMissing(t *testing.T) {
	t.Parallel()
	m := map[string]any{"bytes": []byte("payload")}
	if got := stringFromMap(m, "bytes"); got != "payload" {
		t.Errorf("[]byte string field: got %q", got)
	}
	if got := stringFromMap(m, "missing"); got != "" {
		t.Errorf("missing string field: got %q", got)
	}
}

func TestIntFromMap_Types(t *testing.T) {
	t.Parallel()
	m := map[string]any{
		"int":     int(1),
		"int8":    int8(2),
		"int16":   int16(3),
		"int32":   int32(4),
		"int64":   int64(5),
		"uint":    uint(6),
		"uint8":   uint8(7),
		"uint16":  uint16(8),
		"uint32":  uint32(9),
		"uint64":  uint64(10),
		"float32": float32(11),
		"float64": float64(12),
		"string":  "13",
	}
	for key, want := range map[string]int{"int": 1, "int8": 2, "int16": 3, "int32": 4, "int64": 5, "uint": 6, "uint8": 7, "uint16": 8, "uint32": 9, "uint64": 10, "float32": 11, "float64": 12, "string": 13} {
		got, ok := intFromMap(m, key)
		if !ok || got != want {
			t.Errorf("intFromMap(%s): got (%d,%v), want (%d,true)", key, got, ok, want)
		}
	}
}
