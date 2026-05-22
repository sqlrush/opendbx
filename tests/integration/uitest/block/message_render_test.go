// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

//go:build !windows

package block_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/block"
	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
	"github.com/sqlrush/opendbx/internal/app/cli/render/style"
	"github.com/sqlrush/opendbx/internal/testing/uiinvariant"
	"github.com/sqlrush/opendbx/internal/testing/visualgolden"
)

func TestMessageVisualGolden(t *testing.T) {
	cases := []struct {
		name string
		msg  block.Message
		cols int
	}{
		{name: "MessagePlain", msg: block.Message{Text: "hello world"}, cols: 80},
		{name: "MessageMultiline", msg: block.Message{Text: "hello\nworld"}, cols: 80},
		{
			name: "MessageMixedProseFence",
			msg:  block.Message{Text: "Use fmt.Println:\n```go\nfmt.Println(\"hello\")\n```\nDone."},
			cols: 80,
		},
		{name: "MessageCodeFenceGo", msg: block.Message{Text: "```go\npackage main\nfunc main() {}\n```"}, cols: 80},
		{name: "MessageTruncated", msg: block.Message{Text: strings.Repeat("token ", 24), Truncated: true}, cols: 80},
		{name: "MessageContinued", msg: block.Message{Text: "streamed partial row", Continued: true}, cols: 80},
		{name: "MessageEmpty", msg: block.Message{Empty: true}, cols: 80},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx := ctxDefault(tc.cols)
			buf, err := tc.msg.Render(ctx)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			raw := bufferANSI(t, buf)
			uiinvariant.CheckANSI(t, raw)
			png := visualgolden.Render(t, raw, visualgolden.DefaultTheme())
			if !visualgolden.Update() && !visualFixtureExists(t, "golden") {
				if os.Getenv("BLOCK_VISUAL_REQUIRED") != "" {
					t.Fatalf("missing CC visual fixture for %s (run T-2.5 capture SOP first)", tc.name)
				}
				t.Skipf("missing CC visual fixture for %s; T-2.5 capture pending", tc.name)
			}
			visualgolden.Compare(t, "golden", png, 0.01)
		})
	}
}

func ctxDefault(cols int) block.Context {
	return block.Context{Cols: cols, Theme: block.DefaultTheme{}, Wrap: block.WrapSoft}
}

func bufferANSI(t testing.TB, buf buffer.Buffer) []byte {
	t.Helper()
	cols, rows := buf.Size()
	var b strings.Builder
	cur := style.Style{}
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			c := buf.Cell(x, y)
			if buffer.IsContinuation(c) {
				continue
			}
			if c.St != cur {
				b.WriteString(style.Reset)
				if ansi := c.St.ANSI(); ansi != "" {
					b.WriteString(ansi)
				}
				cur = c.St
			}
			ch := c.Ch
			if ch == 0 {
				ch = ' '
			}
			b.WriteRune(ch)
		}
		b.WriteString(style.Reset)
		cur = style.Style{}
		if y+1 < rows {
			b.WriteByte('\n')
		}
	}
	b.WriteString(style.Reset)
	return []byte(b.String())
}

func visualFixtureExists(t testing.TB, name string) bool {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(file), "testdata", "visual", t.Name(), name+".png")
	if _, err := os.Stat(path); err == nil {
		return true
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat visual fixture %s: %v", path, err)
	}
	return false
}

func TestBufferANSI_EmitsResetAndText(t *testing.T) {
	buf, err := buffer.NewGrid(5, 1)
	if err != nil {
		t.Fatalf("NewGrid: %v", err)
	}
	buf.SetCell(0, 0, buffer.Cell{Ch: 'x', St: style.Style{FG: style.Palette(8)}})
	got := string(bufferANSI(t, buf))
	if !strings.Contains(got, "x") {
		t.Fatalf("ANSI output missing text: %q", got)
	}
	if !strings.HasSuffix(got, style.Reset) {
		t.Fatalf("ANSI output should end with reset: %q", got)
	}
}
