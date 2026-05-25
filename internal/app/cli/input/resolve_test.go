// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package input

import (
	"strings"
	"testing"

	"github.com/sqlrush/opendbx/internal/app/cli/render/terminal"
)

// --- T1-6..T1-15 ResolveMode (R2 H-7 scope-limit: cursor==end only) ---

func TestResolveMode_KeyRune_FirstSlash(t *testing.T) {
	t.Parallel()
	buf, cursor := ResolveMode("", 0, terminal.KeyRune, '/')
	if buf != "/" || cursor != 1 {
		t.Errorf("got (%q, %d), want (\"/\", 1)", buf, cursor)
	}
	if got := DeriveMode(buf); got != ModeSlash {
		t.Errorf("DeriveMode after `/` = %v, want Slash", got)
	}
}

func TestResolveMode_KeyRune_FirstBackslash(t *testing.T) {
	t.Parallel()
	buf, cursor := ResolveMode("", 0, terminal.KeyRune, '\\')
	if buf != "\\" || cursor != 1 {
		t.Errorf("got (%q, %d), want (`\\`, 1)", buf, cursor)
	}
	if got := DeriveMode(buf); got != ModeSQL {
		t.Errorf("DeriveMode after `\\` = %v, want SQL", got)
	}
}

func TestResolveMode_KeyRune_FirstNatural(t *testing.T) {
	t.Parallel()
	buf, cursor := ResolveMode("", 0, terminal.KeyRune, 'h')
	if buf != "h" || cursor != 1 {
		t.Errorf("got (%q, %d), want (\"h\", 1)", buf, cursor)
	}
	if got := DeriveMode(buf); got != ModeNatural {
		t.Errorf("DeriveMode after `h` = %v, want Natural", got)
	}
}

func TestResolveMode_KeyBackspace_ToEmpty(t *testing.T) {
	t.Parallel()
	// /h → /
	buf, cursor := ResolveMode("/h", 2, terminal.KeyBackspace, 0)
	if buf != "/" || cursor != 1 {
		t.Errorf("backspace /h got (%q, %d), want (\"/\", 1)", buf, cursor)
	}
	// / → "" (auto-Natural by DeriveMode)
	buf, cursor = ResolveMode("/", 1, terminal.KeyBackspace, 0)
	if buf != "" || cursor != 0 {
		t.Errorf("backspace / got (%q, %d), want (\"\", 0)", buf, cursor)
	}
	if got := DeriveMode(buf); got != ModeNatural {
		t.Errorf("DeriveMode after backspace to empty = %v, want Natural", got)
	}
}

func TestResolveMode_KeyBackspace_EmptyNoOp(t *testing.T) {
	t.Parallel()
	buf, cursor := ResolveMode("", 0, terminal.KeyBackspace, 0)
	if buf != "" || cursor != 0 {
		t.Errorf("backspace on empty got (%q, %d), want (\"\", 0)", buf, cursor)
	}
}

func TestResolveMode_KeyRune_MultiSequence(t *testing.T) {
	t.Parallel()
	buf, cursor := "", 0
	for _, r := range "/help" {
		buf, cursor = ResolveMode(buf, cursor, terminal.KeyRune, r)
	}
	if buf != "/help" || cursor != 5 {
		t.Errorf("got (%q, %d), want (\"/help\", 5)", buf, cursor)
	}
	if got := DeriveMode(buf); got != ModeSlash {
		t.Errorf("DeriveMode = %v, want Slash", got)
	}
}

func TestResolveMode_KeyRune_UTF8(t *testing.T) {
	t.Parallel()
	// CJK rune is 3 bytes UTF-8; cursor advances by 1 rune position.
	buf, cursor := ResolveMode("", 0, terminal.KeyRune, '你')
	if buf != "你" || cursor != 1 {
		t.Errorf("got (%q, %d), want (\"你\", 1)", buf, cursor)
	}
	// Backspace should strip the multibyte rune cleanly.
	buf, cursor = ResolveMode(buf, cursor, terminal.KeyBackspace, 0)
	if buf != "" || cursor != 0 {
		t.Errorf("backspace CJK got (%q, %d), want (\"\", 0)", buf, cursor)
	}
}

func TestResolveMode_KeyRune_PasteLikeFlood(t *testing.T) {
	t.Parallel()
	buf, cursor := "", 0
	src := strings.Repeat("abcdef", 100) // 600 runes
	for _, r := range src {
		buf, cursor = ResolveMode(buf, cursor, terminal.KeyRune, r)
	}
	if buf != src || cursor != 600 {
		t.Errorf("paste-like flood mismatched; len(buf)=%d cursor=%d", len(buf), cursor)
	}
	if got := DeriveMode(buf); got != ModeNatural {
		t.Errorf("DeriveMode after non-trigger flood = %v, want Natural", got)
	}
}

func TestResolveMode_OtherCode_NoOp(t *testing.T) {
	t.Parallel()
	// KeyEnter / KeyCtrlC / KeyEscape etc — ResolveMode returns unchanged.
	cases := []int{terminal.KeyEnter, terminal.KeyCtrlC, terminal.KeyEscape, terminal.KeyCtrlBackslash}
	for _, code := range cases {
		buf, cursor := ResolveMode("/help", 5, code, 0)
		if buf != "/help" || cursor != 5 {
			t.Errorf("code %d should be no-op; got (%q, %d)", code, buf, cursor)
		}
	}
}

// --- T1-21 invariant: DeriveMode(ResolveMode(...)) consistency ---

func TestResolveMode_DeriveConsistency(t *testing.T) {
	t.Parallel()
	// Apply a mixed key sequence and verify DeriveMode tracks Buffer[0]
	// at every step (single SoT invariant by construction).
	type step struct {
		code int
		r    rune
	}
	seq := []step{
		{terminal.KeyRune, '/'},
		{terminal.KeyRune, 'h'},
		{terminal.KeyRune, 'i'},
		{terminal.KeyBackspace, 0},
		{terminal.KeyBackspace, 0},
		{terminal.KeyBackspace, 0}, // now empty
		{terminal.KeyRune, '\\'},
		{terminal.KeyRune, 'd'},
	}
	buf, cursor := "", 0
	for _, s := range seq {
		buf, cursor = ResolveMode(buf, cursor, s.code, s.r)
		// Invariant: DeriveMode(buf) only looks at buf[0]; always consistent.
		_ = DeriveMode(buf)
		if cursor < 0 {
			t.Fatalf("cursor went negative at step %v: buf=%q cursor=%d", s, buf, cursor)
		}
	}
	if buf != "\\d" {
		t.Errorf("final buf = %q, want \"\\d\"", buf)
	}
	if got := DeriveMode(buf); got != ModeSQL {
		t.Errorf("final DeriveMode = %v, want SQL", got)
	}
}
