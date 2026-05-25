// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

//go:build !windows

package program_test

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

// TestProgramVisualGolden consumes 5 Program CC fixtures parked under
// tests/integration/uitest/program/testdata/visual/Program*/.
//
// Fixture coverage (spec-1.15 D-8):
//
//	ProgramEmptyScrollback — startup; scrollback empty, "> " input row, "opendbx" status
//	ProgramSingleBlock     — one row of scrollback content + input + status
//	ProgramQuitArmed       — input row shows "Press Ctrl+C again to quit" overlay
//	ProgramErrorToast      — ErrorMsg propagated via scheduler.EmitError
//	ProgramResize          — 80×24 → 120×40 layout recompute
//
// 8th independent block-style visual env gate: PROGRAM_VISUAL_REQUIRED=1
// turns missing fixtures into fatal (CI strict); without it tests
// t.Skip (developer-local convenience). Same pattern as the 7 production
// block harnesses (Message / ToolUse / ToolResult / Compact / Markdown /
// Code / Diff).
func TestProgramVisualGolden(t *testing.T) {
	cases := []struct {
		name      string
		cols      int
		rows      int
		quitArmed bool
		text      string // synthetic Scrollback content (single row)
	}{
		{"ProgramEmptyScrollback", 80, 24, false, ""},
		{"ProgramSingleBlock", 80, 24, false, "Hello, opendbx!"},
		{"ProgramQuitArmed", 80, 24, true, ""},
		{"ProgramErrorToast", 80, 24, false, "ErrorMsg: example panic recovered"},
		{"ProgramResize", 120, 40, false, "Resized scene"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			drv := &mockDriver{cols: tc.cols, rows: tc.rows}
			m := &fixtureModel{text: tc.text}
			p := program.New(drv, m)
			grid, err := buffer.NewGrid(tc.cols, tc.rows)
			if err != nil {
				t.Fatalf("NewGrid: %v", err)
			}
			// Drive renderFn directly via the exported test seam.
			program.RunRenderForTest(p, grid, tc.quitArmed)
			raw := gridASCII(grid)
			fixturePath := visualFixturePath(t, tc.name, "golden.png")
			if !visualgolden.Update() && !visualFixtureExists(t, fixturePath) {
				if os.Getenv("PROGRAM_VISUAL_REQUIRED") != "" {
					t.Fatalf("missing CC visual fixture for %s (run capture SOP first)", tc.name)
				}
				t.Skipf("missing CC visual fixture for %s; spec-1.15 capture pending (set PROGRAM_VISUAL_REQUIRED=1 once captured)", tc.name)
			}
			png := visualgolden.Render(t, raw, visualgolden.DefaultTheme())
			visualgolden.CompareFile(t, fixturePath, png, 0.025)
		})
	}
}

// TestProgramVisualGolden_ParkedFixtures verifies the 5 fixture parking
// dirs exist before harness runs.
func TestProgramVisualGolden_ParkedFixtures(t *testing.T) {
	t.Parallel()
	required := []string{
		"ProgramEmptyScrollback",
		"ProgramSingleBlock",
		"ProgramQuitArmed",
		"ProgramErrorToast",
		"ProgramResize",
	}
	for _, name := range required {
		path := visualFixturePath(t, name, "")
		path = strings.TrimSuffix(path, "/")
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			t.Errorf("parked fixture dir missing: %s (run spec-1.15 capture SOP)", path)
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

// --- fixtureModel — single-row scrollback synthesizer ---

type fixtureModel struct {
	text string
}

func (m *fixtureModel) Init() scheduler.Cmd { return nil }
func (m *fixtureModel) Update(msg scheduler.Msg) (program.Model, scheduler.Cmd) {
	return m, nil
}
func (m *fixtureModel) View(cols, rows int) buffer.Buffer {
	b, _ := buffer.NewGrid(cols, rows)
	if m.text == "" || rows <= 0 {
		return b
	}
	for x, r := range []rune(m.text) {
		if x >= cols {
			break
		}
		b.SetCell(x, 0, buffer.Cell{Ch: r})
	}
	return b
}
