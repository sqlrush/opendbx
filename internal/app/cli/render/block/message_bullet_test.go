// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package block

import (
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/buffer"
)

// TestMessage_AssistantBullet: SpeakerAssistant prefixes the first line with
// the speaker bullet and hanging-indents wrapped continuation lines by 2,
// while the glyph appears only ONCE (per-message, not per-line).
func TestMessage_AssistantBullet(t *testing.T) {
	t.Parallel()
	// Force a wrap: narrow cols so the text spans >1 line.
	m := Message{Text: "alpha beta gamma delta", Speaker: SpeakerAssistant}
	buf, err := m.Render(Context{Cols: 12, Rows: 24, Wrap: WrapSoft})
	if err != nil {
		t.Fatalf("render err: %v", err)
	}
	g := buf.(*buffer.Grid)
	_, rows := g.Size()
	if rows < 2 {
		t.Fatalf("expected wrapped (>=2) rows, got %d", rows)
	}
	// line 0 col 0 == bullet; col 1 blank (gutter)
	if got := g.Cell(0, 0).Ch; got != speakerBullet {
		t.Errorf("line0 col0 = %q; want bullet %q", got, speakerBullet)
	}
	// content begins at col 2
	if got := g.Cell(2, 0).Ch; got != 'a' {
		t.Errorf("line0 col2 = %q; want 'a' (content offset by bullet)", got)
	}
	// continuation lines have NO bullet at col 0 (blank gutter)
	for y := 1; y < rows; y++ {
		if got := g.Cell(0, y).Ch; got == speakerBullet {
			t.Errorf("continuation row %d unexpectedly has a bullet", y)
		}
	}
}

// TestMessage_UserAndNonePlain: SpeakerUser / SpeakerNone render identically
// (plain text, no bullet, no "> " prefix).
func TestMessage_UserAndNonePlain(t *testing.T) {
	t.Parallel()
	for _, sp := range []SpeakerKind{SpeakerNone, SpeakerUser} {
		m := Message{Text: "hello", Speaker: sp}
		buf, _ := m.Render(Context{Cols: 40, Rows: 24})
		g := buf.(*buffer.Grid)
		if g.Cell(0, 0).Ch != 'h' {
			t.Errorf("speaker %d: col0 = %q; want plain 'h'", sp, g.Cell(0, 0).Ch)
		}
	}
}

// TestMessage_AssistantBulletEmptyNoOrphan: an empty-text assistant message
// renders 0 rows (no orphan bullet on a blank line).
func TestMessage_AssistantBulletEmptyNoOrphan(t *testing.T) {
	t.Parallel()
	// Both empty-text AND Empty=true (thinking placeholder) must yield 0 rows
	// — no orphan ⏺ bullet (go-reviewer M-3 / code-reviewer MED-2).
	cases := []Message{
		{Text: "", Speaker: SpeakerAssistant},
		{Empty: true, Speaker: SpeakerAssistant},
	}
	for _, m := range cases {
		buf, err := m.Render(Context{Cols: 40, Rows: 24})
		if err != nil {
			t.Fatalf("render err: %v", err)
		}
		if _, rows := buf.Size(); rows != 0 {
			t.Errorf("%+v rows = %d; want 0 (no orphan bullet)", m, rows)
		}
	}
}
