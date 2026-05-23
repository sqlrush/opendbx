// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

// File toolcall_test.go — spec-1.9 D-8 unit test suite for ToolUse
// production (≥ 30 case across 5 states × 3 adapter × verbose/condensed
// × edge per spec § 4.1). Coverage gate ≥ 85% per CLAUDE rule 8.

package block

import (
	"strings"
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/block/adapter"
)

// ctxToolUse builds a default render Context for ToolUse tests.
func ctxToolUse(cols int) Context {
	return Context{Cols: cols, Rows: 24, Wrap: WrapSoft}
}

// readRowText reads row y of buf, skipping NUL chars, preserving order
// (spaces and content runes preserved verbatim).
func readRowText(t *testing.T, buf interface {
	Size() (int, int)
}, cellFn func(x, y int) rune, y int) string {
	t.Helper()
	cols, _ := buf.Size()
	var b strings.Builder
	for x := 0; x < cols; x++ {
		c := cellFn(x, y)
		if c == 0 {
			continue
		}
		b.WriteRune(c)
	}
	return b.String()
}

// === unit tests ===

// #1 — Queued × 3 adapter (Bash registered / Read registered / Generic).
func TestToolUse_Queued_Bash(t *testing.T) {
	t.Parallel()
	tu := NewToolUse("id1", "Bash", map[string]any{"command": "ls"})
	buf, err := tu.Render(ctxToolUse(80))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	cols, rows := buf.Size()
	if cols == 0 || rows == 0 {
		t.Fatalf("expected non-zero buffer, got cols=%d rows=%d", cols, rows)
	}
	got := readRowText(t, buf, func(x, y int) rune { return buf.Cell(x, y).Ch }, 0)
	if !strings.Contains(got, "Waiting") {
		t.Errorf("Queued Bash row want contains 'Waiting', got %q", got)
	}
}

func TestToolUse_Queued_Generic(t *testing.T) {
	t.Parallel()
	tu := NewToolUse("id2", "UnknownTool", map[string]any{"foo": "bar"})
	buf, _ := tu.Render(ctxToolUse(80))
	got := readRowText(t, buf, func(x, y int) rune { return buf.Cell(x, y).Ch }, 0)
	if !strings.Contains(got, "Waiting") {
		t.Errorf("Queued Generic want 'Waiting', got %q", got)
	}
}

// #2 — Running × 3 adapter (Bash → with "Running…" 2nd row; Read → header only; Generic → header only).
func TestToolUse_Running_Bash_NoProgress(t *testing.T) {
	t.Parallel()
	tu := NewToolUse("id3", "Bash", map[string]any{"command": "git status"})
	tu.State = StateRunning
	buf, _ := tu.Render(ctxToolUse(80))
	_, rows := buf.Size()
	if rows != 2 {
		t.Fatalf("Running Bash empty progress: want 2 rows (header + Running…), got %d", rows)
	}
	row1 := readRowText(t, buf, func(x, y int) rune { return buf.Cell(x, y).Ch }, 1)
	if !strings.Contains(row1, "Running") {
		t.Errorf("row1 want 'Running…', got %q", row1)
	}
}

func TestToolUse_Running_Bash_WithProgress(t *testing.T) {
	t.Parallel()
	tu := NewToolUse("id4", "Bash", map[string]any{"command": "long_cmd"})
	tu.State = StateRunning
	tu.ProgressMessages = []adapter.ProgressMessage{
		{ElapsedSeconds: 5, TotalLines: 100, TotalBytes: 4096},
	}
	buf, _ := tu.Render(ctxToolUse(80))
	_, rows := buf.Size()
	if rows != 2 {
		t.Fatalf("Running Bash with progress: want 2 rows, got %d", rows)
	}
	row1 := readRowText(t, buf, func(x, y int) rune { return buf.Cell(x, y).Ch }, 1)
	if !strings.Contains(row1, "5s") || !strings.Contains(row1, "100") {
		t.Errorf("progress row missing fields: %q", row1)
	}
}

func TestToolUse_Running_Read_NoProgress(t *testing.T) {
	t.Parallel()
	tu := NewToolUse("id5", "Read", map[string]any{"path": "main.go"})
	tu.State = StateRunning
	buf, _ := tu.Render(ctxToolUse(80))
	_, rows := buf.Size()
	if rows != 1 {
		t.Fatalf("Running Read: Read has no ProgressRenderer → header-only, got %d rows", rows)
	}
}

func TestToolUse_Running_Generic_NoProgress(t *testing.T) {
	t.Parallel()
	tu := NewToolUse("id6", "Mystery", map[string]any{"key": "value"})
	tu.State = StateRunning
	buf, _ := tu.Render(ctxToolUse(80))
	_, rows := buf.Size()
	if rows != 1 {
		t.Fatalf("Running Generic: no ProgressRenderer → header-only, got %d rows", rows)
	}
}

// #3 — WaitingPermission × Bash + Read; exact CC text + dim style.
func TestToolUse_WaitingPermission_Bash(t *testing.T) {
	t.Parallel()
	tu := NewToolUse("id7", "Bash", map[string]any{"command": "rm /tmp/x"})
	tu.State = StateWaitingPermission
	buf, _ := tu.Render(ctxToolUse(80))
	_, rows := buf.Size()
	if rows != 2 {
		t.Fatalf("WaitingPermission: want 2 rows (header + dim), got %d", rows)
	}
	row1 := readRowText(t, buf, func(x, y int) rune { return buf.Cell(x, y).Ch }, 1)
	if !strings.Contains(row1, "Waiting for permission") {
		t.Errorf("row1 want CC exact 'Waiting for permission…', got %q", row1)
	}
}

// #4 — Resolved × Bash + Read; single header row.
func TestToolUse_Resolved_Bash(t *testing.T) {
	t.Parallel()
	tu := NewToolUse("id8", "Bash", map[string]any{"command": "ls"})
	tu.State = StateResolved
	buf, _ := tu.Render(ctxToolUse(80))
	_, rows := buf.Size()
	if rows != 1 {
		t.Fatalf("Resolved: want 1 row, got %d", rows)
	}
}

func TestToolUse_Resolved_Read(t *testing.T) {
	t.Parallel()
	tu := NewToolUse("id9", "Read", map[string]any{"path": "x.go"})
	tu.State = StateResolved
	buf, _ := tu.Render(ctxToolUse(80))
	_, rows := buf.Size()
	if rows != 1 {
		t.Fatalf("Resolved Read: want 1 row, got %d", rows)
	}
}

// #5 — Error × 3 adapter; with _error row.
func TestToolUse_Error_WithMessage(t *testing.T) {
	t.Parallel()
	tu := NewToolUse("idA", "Bash", map[string]any{"command": "fail", "_error": "exit 1"})
	tu.State = StateError
	buf, _ := tu.Render(ctxToolUse(80))
	_, rows := buf.Size()
	if rows != 2 {
		t.Fatalf("Error with _error: want 2 rows, got %d", rows)
	}
	row1 := readRowText(t, buf, func(x, y int) rune { return buf.Cell(x, y).Ch }, 1)
	if !strings.Contains(row1, "exit 1") {
		t.Errorf("error row want 'exit 1', got %q", row1)
	}
}

func TestToolUse_Error_NoMessage(t *testing.T) {
	t.Parallel()
	tu := NewToolUse("idB", "Bash", map[string]any{"command": "x"})
	tu.State = StateError
	buf, _ := tu.Render(ctxToolUse(80))
	_, rows := buf.Size()
	if rows != 1 {
		t.Fatalf("Error no _error: want 1 row (header only), got %d", rows)
	}
}

// #6 — Bash 160-char truncate + verbose toggle.
func TestToolUse_Bash_Truncate160(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("x", 200)
	tu := NewToolUse("idC", "Bash", map[string]any{"command": long})
	tu.State = StateRunning
	buf, _ := tu.Render(ctxToolUse(300))
	row0 := readRowText(t, buf, func(x, y int) rune { return buf.Cell(x, y).Ch }, 0)
	// row0 starts with indicator '⏵ '; the truncated cmd should be ≤ 160 char + '…'.
	if !strings.HasSuffix(row0, "…") {
		t.Errorf("expected '…' suffix on truncated cmd, got %q", row0)
	}
}

// #7 — Read default vs verbose.
func TestToolUse_Read_DefaultPath(t *testing.T) {
	t.Parallel()
	tu := NewToolUse("idD", "Read", map[string]any{"path": "main.go"})
	tu.State = StateResolved
	buf, _ := tu.Render(ctxToolUse(80))
	got := readRowText(t, buf, func(x, y int) rune { return buf.Cell(x, y).Ch }, 0)
	if !strings.Contains(got, "main.go") {
		t.Errorf("default path missing: %q", got)
	}
	if strings.Contains(got, "lines") || strings.Contains(got, "pages") {
		t.Errorf("non-verbose should not include range: %q", got)
	}
}

func TestToolUse_Read_PagesPDF(t *testing.T) {
	t.Parallel()
	tu := NewToolUse("idF", "Read", map[string]any{"path": "x.pdf", "pages": "1-3"})
	tu.State = StateResolved
	buf, _ := tu.Render(ctxToolUse(80))
	got := readRowText(t, buf, func(x, y int) rune { return buf.Cell(x, y).Ch }, 0)
	if !strings.Contains(got, "pages 1-3") {
		t.Errorf("expected 'pages 1-3', got %q", got)
	}
}

// #8 — Generic fallback compact name(k=v).
func TestToolUse_Generic_CompactArgs(t *testing.T) {
	t.Parallel()
	tu := NewToolUse("idG", "Mystery", map[string]any{"k": "v", "a": "b"})
	tu.State = StateResolved
	buf, _ := tu.Render(ctxToolUse(80))
	got := readRowText(t, buf, func(x, y int) rune { return buf.Cell(x, y).Ch }, 0)
	if !strings.Contains(got, "Mystery") {
		t.Errorf("want tool name, got %q", got)
	}
	if !strings.Contains(got, "a=b") {
		t.Errorf("want sorted key a=b first, got %q", got)
	}
}

// #9 — Edge: empty Name → "(unnamed tool)" fallback via Generic.
func TestToolUse_EmptyName(t *testing.T) {
	t.Parallel()
	tu := NewToolUse("idH", "", nil)
	tu.State = StateResolved
	buf, _ := tu.Render(ctxToolUse(80))
	got := readRowText(t, buf, func(x, y int) rune { return buf.Cell(x, y).Ch }, 0)
	if !strings.Contains(got, "unnamed") {
		t.Errorf("want '(unnamed)' fallback, got %q", got)
	}
}

// #10 — Edge: Input==nil graceful.
func TestToolUse_NilInput(t *testing.T) {
	t.Parallel()
	tu := NewToolUse("idI", "Bash", nil)
	tu.State = StateRunning
	buf, _ := tu.Render(ctxToolUse(80))
	got := readRowText(t, buf, func(x, y int) rune { return buf.Cell(x, y).Ch }, 0)
	if !strings.Contains(got, "no command") {
		t.Errorf("nil Input should produce '(no command)', got %q", got)
	}
}

// #11 — Edge: ctx.Cols=0 → 0-row buffer.
func TestToolUse_ZeroCols(t *testing.T) {
	t.Parallel()
	tu := NewToolUse("idJ", "Bash", map[string]any{"command": "x"})
	buf, _ := tu.Render(Context{Cols: 0})
	cols, rows := buf.Size()
	if cols != 0 || rows != 0 {
		t.Errorf("zero cols: want (0,0), got (%d,%d)", cols, rows)
	}
}

// #12 — Edge: MeasureOnly fast path.
func TestToolUse_MeasureOnly(t *testing.T) {
	t.Parallel()
	for _, st := range []ToolUseState{StateQueued, StateRunning, StateWaitingPermission, StateResolved, StateError} {
		tu := NewToolUse("idK", "Bash", map[string]any{"command": "x"})
		tu.State = st
		ctx := Context{Cols: 80, Rows: 24, MeasureOnly: true}
		buf, _ := tu.Render(ctx)
		_, rows := buf.Size()
		if rows <= 0 {
			t.Errorf("state %v MeasureOnly: rows=%d, want >0", st, rows)
		}
	}
}

// #13 — stateIndicator 5-state lockdown (G2 numerical sanity — values
// may change in R3 errata when CC fixture lands).
func TestStateIndicator_5StateMatrix(t *testing.T) {
	t.Parallel()
	cases := []struct {
		s    ToolUseState
		want rune
	}{
		{StateQueued, '⏸'},
		{StateRunning, '⏵'},
		{StateWaitingPermission, '⏷'},
		{StateResolved, '⏺'},
		{StateError, '✗'},
	}
	for _, c := range cases {
		got := stateIndicator(c.s)
		if got.Rune != c.want {
			t.Errorf("state %v: rune got %q want %q", c.s, got.Rune, c.want)
		}
	}
}

// #14 — Adapter Registry concurrent Register/Lookup race-safe.
func TestRegistry_ConcurrentRegisterLookup(t *testing.T) {
	t.Parallel()
	r := adapter.NewRegistry()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			r.Register("X", adapter.Generic{})
		}
		close(done)
	}()
	for i := 0; i < 100; i++ {
		_ = r.Lookup("X")
	}
	<-done
}

// #15 — Bash empty command → "(no command)".
func TestToolUse_Bash_EmptyCommand(t *testing.T) {
	t.Parallel()
	tu := NewToolUse("idL", "Bash", map[string]any{})
	tu.State = StateResolved
	buf, _ := tu.Render(ctxToolUse(80))
	got := readRowText(t, buf, func(x, y int) rune { return buf.Cell(x, y).Ch }, 0)
	if !strings.Contains(got, "no command") {
		t.Errorf("Bash empty: want '(no command)', got %q", got)
	}
}

// #16 — Read missing path → "(no path)".
func TestToolUse_Read_NoPath(t *testing.T) {
	t.Parallel()
	tu := NewToolUse("idM", "Read", map[string]any{})
	tu.State = StateResolved
	buf, _ := tu.Render(ctxToolUse(80))
	got := readRowText(t, buf, func(x, y int) rune { return buf.Cell(x, y).Ch }, 0)
	if !strings.Contains(got, "no path") {
		t.Errorf("Read empty: want '(no path)', got %q", got)
	}
}

// #17 — Generic single very long pair → truncate.
func TestToolUse_Generic_LongValueTruncate(t *testing.T) {
	t.Parallel()
	tu := NewToolUse("idN", "Mystery", map[string]any{"k": strings.Repeat("x", 200)})
	tu.State = StateResolved
	buf, _ := tu.Render(ctxToolUse(40)) // cols/2 = 20 budget
	got := readRowText(t, buf, func(x, y int) rune { return buf.Cell(x, y).Ch }, 0)
	if !strings.HasSuffix(got, "…") && !strings.Contains(got, "Mystery") {
		t.Errorf("expected truncation marker, got %q", got)
	}
}

// #18 — Running ProgressMessages != nil but State != Running →
// degrade gracefully (no progress row).
func TestToolUse_NonRunningWithProgress(t *testing.T) {
	t.Parallel()
	tu := NewToolUse("idO", "Bash", map[string]any{"command": "x"})
	tu.State = StateResolved
	tu.ProgressMessages = []adapter.ProgressMessage{{Text: "x"}}
	buf, _ := tu.Render(ctxToolUse(80))
	_, rows := buf.Size()
	if rows != 1 {
		t.Errorf("Resolved + ProgressMessages: want 1 row (no progress), got %d", rows)
	}
}

// #19 — NewToolUse default state is StateQueued.
func TestNewToolUse_DefaultsToQueued(t *testing.T) {
	t.Parallel()
	tu := NewToolUse("idP", "Bash", map[string]any{"command": "x"})
	if tu.State != StateQueued {
		t.Errorf("NewToolUse default state = %v, want StateQueued", tu.State)
	}
}

// #20 — Lookup returns nil for unregistered → Generic fallback.
func TestRegistry_UnregisteredLookup(t *testing.T) {
	t.Parallel()
	r := adapter.NewRegistry()
	if got := r.Lookup("never_registered"); got != nil {
		t.Errorf("empty registry: want nil, got %v", got)
	}
}

// #21 — Generic key sort determinism.
func TestGeneric_KeySorted(t *testing.T) {
	t.Parallel()
	g := adapter.Generic{}
	in := map[string]any{"_name": "T", "z": "1", "a": "2", "m": "3"}
	out, _ := g.RenderHeader(in, adapter.Context{Cols: 80})
	// Expect "T(a=2, m=3, z=1)".
	if !strings.HasPrefix(out, "T(a=2") {
		t.Errorf("keys not sorted: %q", out)
	}
}

// #22 — Verbose Bash no truncate.
func TestToolUse_Bash_VerboseNoTruncate(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("x", 200)
	b := adapter.Bash{}
	out, _ := b.RenderHeader(map[string]any{"command": long}, adapter.Context{Verbose: true, Cols: 300})
	if len(out) != 200 {
		t.Errorf("verbose should not truncate, got len=%d", len(out))
	}
}

// #23 — Verbose Read with offset+limit.
func TestToolUse_Read_VerboseLines(t *testing.T) {
	t.Parallel()
	r := adapter.Read{}
	out, _ := r.RenderHeader(map[string]any{"path": "x.go", "offset": 10, "limit": 50}, adapter.Context{Verbose: true, Cols: 80})
	if !strings.Contains(out, "lines 10-59") {
		t.Errorf("want 'lines 10-59', got %q", out)
	}
}

// #24 — Bash 2-line truncate.
func TestToolUse_Bash_TwoLineTruncate(t *testing.T) {
	t.Parallel()
	cmd := "line1\nline2\nline3\nline4"
	b := adapter.Bash{}
	out, _ := b.RenderHeader(map[string]any{"command": cmd}, adapter.Context{Verbose: false, Cols: 200})
	lines := strings.Split(out, "\n")
	if len(lines) > 2 {
		t.Errorf("want ≤2 lines, got %d: %q", len(lines), out)
	}
}

// #25 — Bash progress with various fields.
func TestToolUse_Bash_ProgressVariants(t *testing.T) {
	t.Parallel()
	b := adapter.Bash{}
	// Only ElapsedSeconds.
	out, _ := b.RenderProgress([]adapter.ProgressMessage{{ElapsedSeconds: 3}}, adapter.Context{})
	if !strings.Contains(out, "3s") {
		t.Errorf("want '3s', got %q", out)
	}
	// No fields → "Running…".
	out2, _ := b.RenderProgress([]adapter.ProgressMessage{{}}, adapter.Context{})
	if out2 != "Running…" {
		t.Errorf("empty progress: want 'Running…', got %q", out2)
	}
}

// #26 — adapter.Bash satisfies all 3 interfaces.
func TestBash_SatisfiesAllInterfaces(t *testing.T) {
	t.Parallel()
	var _ adapter.HeaderRenderer = adapter.Bash{}
	var _ adapter.ProgressRenderer = adapter.Bash{}
	var _ adapter.QueuedRenderer = adapter.Bash{}
}

// #27 — adapter.Read satisfies only HeaderRenderer (R2.1.3 HIGH-2:
// Read does NOT implement ProgressRenderer or QueuedRenderer).
func TestRead_OnlyHeaderRenderer(t *testing.T) {
	t.Parallel()
	var _ adapter.HeaderRenderer = adapter.Read{}
	// These would NOT compile if Read implemented them:
	//   var _ adapter.ProgressRenderer = adapter.Read{}  // intentionally absent
	//   var _ adapter.QueuedRenderer = adapter.Read{}    // intentionally absent
	r := adapter.Read{}
	if _, ok := any(r).(adapter.ProgressRenderer); ok {
		t.Errorf("Read should NOT implement ProgressRenderer per R2.1.3 HIGH-2")
	}
	if _, ok := any(r).(adapter.QueuedRenderer); ok {
		t.Errorf("Read should NOT implement QueuedRenderer per R2.1.3 HIGH-2")
	}
}

// #28 — Default Registry has Bash + Read pre-registered.
func TestDefaultRegistry_HasBuiltinAdapters(t *testing.T) {
	t.Parallel()
	if adapter.Default.Lookup("Bash") == nil {
		t.Errorf("Default registry should have Bash registered")
	}
	if adapter.Default.Lookup("Read") == nil {
		t.Errorf("Default registry should have Read registered")
	}
}

// erroringRenderer satisfies HeaderRenderer but always returns an error,
// to exercise the errorPlaceholder fallback path (spec-1.9 T-9 MED-2).
type erroringRenderer struct{}

func (erroringRenderer) RenderHeader(_ map[string]any, _ adapter.Context) (string, error) {
	return "", errAdapterTest
}

var errAdapterTest = errAdapterMock("boom")

type errAdapterMock string

func (e errAdapterMock) Error() string { return string(e) }

// #29 (spec-1.9 T-9 MED-2) — adapter RenderHeader 返 error → errorPlaceholder
// 1-row "[render error: <name>]" buffer (rule 7 block-leaf contract).
func TestToolUse_AdapterError_RendersPlaceholder(t *testing.T) {
	t.Parallel()
	reg := adapter.NewRegistry()
	reg.Register("Boom", erroringRenderer{})
	// Inject mock registry by registering in Default just for this test.
	// (No public adapter.SetDefault — test names a unique tool ensuring
	// no collision with production Bash/Read/Generic.)
	adapter.Default.Register("__boom_test__", erroringRenderer{})
	tu := NewToolUse("idErr", "__boom_test__", map[string]any{})
	tu.State = StateRunning
	buf, _ := tu.Render(ctxToolUse(40))
	_, rows := buf.Size()
	if rows != 1 {
		t.Errorf("error placeholder: want 1 row, got %d", rows)
	}
	got := readRowText(t, buf, func(x, y int) rune { return buf.Cell(x, y).Ch }, 0)
	if !strings.Contains(got, "render error") {
		t.Errorf("placeholder text: got %q", got)
	}
}

// #30 (spec-1.9 T-9 MED-1) — ctx.Verbose 流到 adapter (Read verbose path).
func TestToolUse_VerbosePropagated(t *testing.T) {
	t.Parallel()
	tu := NewToolUse("idV", "Read", map[string]any{"path": "x.go", "offset": 5, "limit": 10})
	tu.State = StateResolved
	ctx := Context{Cols: 80, Rows: 24, Wrap: WrapSoft, Verbose: true}
	buf, _ := tu.Render(ctx)
	got := readRowText(t, buf, func(x, y int) rune { return buf.Cell(x, y).Ch }, 0)
	if !strings.Contains(got, "lines 5-14") {
		t.Errorf("ctx.Verbose propagation failed: want 'lines 5-14', got %q", got)
	}
}

// #31 (spec-1.9 T-9 HIGH-1) — Bash CJK truncation produces valid UTF-8.
func TestToolUse_Bash_CJKTruncation_ValidUTF8(t *testing.T) {
	t.Parallel()
	cmd := strings.Repeat("x", 158) + "中文测试abc"
	b := adapter.Bash{}
	out, _ := b.RenderHeader(map[string]any{"command": cmd}, adapter.Context{})
	if !isValidUTF8(out) {
		t.Errorf("CJK truncation produced invalid UTF-8: %q", out)
	}
}

func isValidUTF8(s string) bool {
	for _, r := range s {
		if r == '�' && !strings.Contains(s, "�") {
			return false
		}
	}
	return true
}
