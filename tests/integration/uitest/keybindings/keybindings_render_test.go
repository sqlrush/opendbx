// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

//go:build !windows

package keybindings_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/program"
	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
	"github.com/sqlrush/opendbx/internal/app/cli/render/scheduler"
	"github.com/sqlrush/opendbx/internal/app/cli/render/style"
	"github.com/sqlrush/opendbx/internal/app/cli/render/terminal"
	"github.com/sqlrush/opendbx/internal/testing/visualgolden"
)

// TestKeybindingsVisualGolden consumes 5 cursor-edit / history CC
// fixtures parked under
// tests/integration/uitest/keybindings/testdata/visual/Keybindings*/.
//
// Fixture coverage (spec-1.17 D-8):
//
//	KeybindingsCursorMid     — "> hel_lo" cursor mid-buffer (rune pos 3)
//	KeybindingsCursorHome    — "> _hello" cursor at start (Ctrl+A)
//	KeybindingsHistoryNav    — input row shows a recalled history entry
//	KeybindingsKeyDelete     — "> hel_o" after forward delete at cursor
//	KeybindingsSubmitHistory — empty input row + "> hello" scrollback line
//
// 10th independent block-style visual env gate: KEYBINDINGS_VISUAL_REQUIRED=1
// turns missing fixtures into fatal (CI strict); without it tests t.Skip
// (developer-local convenience). Same pattern as the 9 prior gates
// (Message / ToolUse / ToolResult / Compact / Markdown / Code / Diff /
// Program / Input).
func TestKeybindingsVisualGolden(t *testing.T) {
	cases := []struct {
		name   string
		buf    string
		cursor int
		log    string // synthetic scrollback line (SubmitHistory)
		cols   int
		rows   int
	}{
		{"KeybindingsCursorMid", "hello", 3, "", 80, 24},
		{"KeybindingsCursorHome", "hello", 0, "", 80, 24},
		{"KeybindingsHistoryNav", "select * from t", 15, "", 80, 24},
		{"KeybindingsKeyDelete", "helo", 3, "", 80, 24},
		{"KeybindingsSubmitHistory", "", 0, "hello", 80, 24},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			drv := &mockDriver{cols: tc.cols, rows: tc.rows}
			m := &fixtureModel{buf: tc.buf, cursor: tc.cursor, log: tc.log}
			p := program.New(drv, m)
			grid, err := buffer.NewGrid(tc.cols, tc.rows)
			if err != nil {
				t.Fatalf("NewGrid: %v", err)
			}
			program.RunRenderForTest(p, grid, false)
			raw := gridASCII(grid)
			fixturePath := visualFixturePath(t, tc.name, "golden.png")
			if !visualgolden.Update() && !visualFixtureExists(t, fixturePath) {
				if os.Getenv("KEYBINDINGS_VISUAL_REQUIRED") != "" {
					t.Fatalf("missing CC visual fixture for %s (run capture SOP first)", tc.name)
				}
				t.Skipf("missing CC visual fixture for %s; spec-1.17 capture pending (set KEYBINDINGS_VISUAL_REQUIRED=1 once captured)", tc.name)
			}
			png := visualgolden.Render(t, raw, visualgolden.DefaultTheme())
			visualgolden.CompareFile(t, fixturePath, png, 0.025)
		})
	}
}

// TestKeybindingsVisualGolden_ParkedFixtures verifies the 5 fixture
// parking dirs exist before harness runs.
func TestKeybindingsVisualGolden_ParkedFixtures(t *testing.T) {
	t.Parallel()
	required := []string{
		"KeybindingsCursorMid",
		"KeybindingsCursorHome",
		"KeybindingsHistoryNav",
		"KeybindingsKeyDelete",
		"KeybindingsSubmitHistory",
	}
	for _, name := range required {
		path := visualFixturePath(t, name, "")
		path = strings.TrimSuffix(path, "/")
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			t.Errorf("parked fixture dir missing: %s (run spec-1.17 capture SOP)", path)
		}
	}
}

// --- harness helpers ---

func visualFixturePath(t testing.TB, fixture, name string) string {
	t.Helper()
	base := filepath.Join("testdata", "visual", fixture)
	if name == "" {
		return base + "/"
	}
	return filepath.Join(base, name)
}

func visualFixtureExists(t testing.TB, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	return err == nil
}

func gridASCII(buf buffer.Buffer) []byte {
	cols, rows := buf.Size()
	var b bytes.Buffer
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			c := buf.Cell(x, y)
			if c.Ch == 0 || c.Ch < 0 {
				b.WriteByte(' ')
				continue
			}
			b.WriteRune(c.Ch)
		}
		b.WriteByte('\n')
	}
	return b.Bytes()
}

// --- mockDriver implements terminal.Driver minimally for fixture render ---

type mockDriver struct {
	cols, rows int
}

func (d *mockDriver) Init() error                               { return nil }
func (d *mockDriver) Fini()                                     {}
func (d *mockDriver) Show()                                     {}
func (d *mockDriver) Sync()                                     {}
func (d *mockDriver) Clear()                                    {}
func (d *mockDriver) Size() (int, int)                          { return d.cols, d.rows }
func (d *mockDriver) SetCell(x, y int, ch rune, st style.Style) {}
func (d *mockDriver) PollEvent(ctx context.Context) (terminal.Event, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
func (d *mockDriver) PostEvent(terminal.Event) error { return nil }
func (d *mockDriver) Resize(c, r int)                { d.cols, d.rows = c, r }

// --- fixtureModel — exposes buffer + cursor for the input row, plus an
// optional single scrollback log line for the SubmitHistory fixture.

type fixtureModel struct {
	buf    string
	cursor int
	log    string
}

func (m *fixtureModel) Init() scheduler.Cmd { return nil }
func (m *fixtureModel) Update(msg scheduler.Msg) (program.Model, scheduler.Cmd) {
	return m, nil
}
func (m *fixtureModel) View(cols, rows int) buffer.Buffer {
	b, _ := buffer.NewGrid(cols, rows)
	if m.log == "" || rows <= 0 {
		return b
	}
	line := "> " + m.log
	for x, r := range []rune(line) {
		if x >= cols {
			break
		}
		b.SetCell(x, rows-1, buffer.Cell{Ch: r})
	}
	return b
}

// InputState drives program.paintInputRow — buffer + cursor position
// produce the "_" cursor glyph at the rune-relative cell (spec-1.16 R3
// cursor field render + spec-1.17 cursor-anywhere edit).
func (m *fixtureModel) InputState() program.InputState {
	return program.InputState{Buffer: m.buf, Cursor: m.cursor}
}

func (m *fixtureModel) StatusSegments() []program.StatusSegment {
	return []program.StatusSegment{{Text: "opendbx-demo"}}
}
