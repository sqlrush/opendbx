// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

//go:build !windows

package input_test

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

// TestInputModeVisualGolden consumes 5 input-mode CC fixtures parked
// under tests/integration/uitest/input/testdata/visual/Mode*/.
//
// Fixture coverage (spec-1.16 D-8):
//
//	ModeNatural       — "> hello world_" Natural mode
//	ModeSlash         — "/help_" Slash mode (no "> " prompt)
//	ModeSQL           — "\select * from t_" SQL mode
//	InputModeStatusSegment — status line "opendbx slash" (mode segment appended)
//	InputModeSwitchTrace   — natural → slash → SQL → natural sequence
//
// 9th independent block-style visual env gate: INPUT_VISUAL_REQUIRED=1
// turns missing fixtures into fatal (CI strict); without it tests
// t.Skip (developer-local convenience). Same pattern as the 8 prior
// gates (Message / ToolUse / ToolResult / Compact / Markdown / Code /
// Diff / Program).
func TestInputModeVisualGolden(t *testing.T) {
	cases := []struct {
		name string
		buf  string
		cols int
		rows int
	}{
		{"InputModeNatural", "hello world", 80, 24},
		{"InputModeSlash", "/help", 80, 24},
		{"InputModeSQL", "\\select * from t", 80, 24},
		{"InputModeStatusSegment", "/model", 80, 24},
		{"InputModeSwitchTrace", "\\d", 80, 24},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			drv := &mockDriver{cols: tc.cols, rows: tc.rows}
			m := &fixtureModel{buf: tc.buf}
			p := program.New(drv, m)
			grid, err := buffer.NewGrid(tc.cols, tc.rows)
			if err != nil {
				t.Fatalf("NewGrid: %v", err)
			}
			program.RunRenderForTest(p, grid, false)
			raw := gridASCII(grid)
			fixturePath := visualFixturePath(t, tc.name, "golden.png")
			if !visualgolden.Update() && !visualFixtureExists(t, fixturePath) {
				if os.Getenv("INPUT_VISUAL_REQUIRED") != "" {
					t.Fatalf("missing CC visual fixture for %s (run capture SOP first)", tc.name)
				}
				t.Skipf("missing CC visual fixture for %s; spec-1.16 capture pending (set INPUT_VISUAL_REQUIRED=1 once captured)", tc.name)
			}
			png := visualgolden.Render(t, raw, visualgolden.DefaultTheme())
			visualgolden.CompareFile(t, fixturePath, png, 0.025)
		})
	}
}

// TestInputModeVisualGolden_ParkedFixtures verifies the 5 fixture
// parking dirs exist before harness runs.
func TestInputModeVisualGolden_ParkedFixtures(t *testing.T) {
	t.Parallel()
	required := []string{
		"InputModeNatural",
		"InputModeSlash",
		"InputModeSQL",
		"InputModeStatusSegment",
		"InputModeSwitchTrace",
	}
	for _, name := range required {
		path := visualFixturePath(t, name, "")
		path = strings.TrimSuffix(path, "/")
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			t.Errorf("parked fixture dir missing: %s (run spec-1.16 capture SOP)", path)
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
			// R4 N-4: Ch == 0 is empty cell; Ch < 0 is the
			// buffer.WideContinuation sentinel (-1) for wide-rune cells.
			// Both render as space in the ASCII transcript.
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

// --- mockDriver ---

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

// --- fixtureModel: implements program.Model + InputModel for mode tests ---

type fixtureModel struct {
	buf string
}

func (m *fixtureModel) Init() scheduler.Cmd { return nil }
func (m *fixtureModel) Update(msg scheduler.Msg) (program.Model, scheduler.Cmd) {
	return m, nil
}
func (m *fixtureModel) View(cols, rows int) buffer.Buffer {
	b, _ := buffer.NewGrid(cols, rows)
	return b
}
func (m *fixtureModel) InputState() program.InputState {
	return program.InputState{Buffer: m.buf, Cursor: len([]rune(m.buf))}
}
