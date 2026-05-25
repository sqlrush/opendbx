// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package input

import "testing"

// --- T1-1..T1-5 Mode enum ---

func TestInputMode_String(t *testing.T) {
	t.Parallel()
	cases := []struct {
		mode Mode
		want string
	}{
		{ModeNatural, "natural"},
		{ModeSlash, "slash"},
		{ModeSQL, "sql"},
		{Mode(99), "natural"}, // unknown defaults to natural
	}
	for _, c := range cases {
		if got := c.mode.String(); got != c.want {
			t.Errorf("Mode(%d).String() = %q, want %q", c.mode, got, c.want)
		}
	}
}

func TestInputMode_TriggerRune(t *testing.T) {
	t.Parallel()
	cases := []struct {
		mode Mode
		want rune
	}{
		{ModeNatural, 0},
		{ModeSlash, '/'},
		{ModeSQL, '\\'},
	}
	for _, c := range cases {
		if got := c.mode.TriggerRune(); got != c.want {
			t.Errorf("%v.TriggerRune() = %q, want %q", c.mode, got, c.want)
		}
	}
}

func TestInputMode_ZeroValue(t *testing.T) {
	t.Parallel()
	var m Mode
	if m != ModeNatural {
		t.Errorf("zero value Mode = %v, want ModeNatural", m)
	}
}

// --- T1-6..T1-15 DeriveMode (R2 C2 ★A single SoT) ---

func TestDeriveMode(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		buffer string
		want   Mode
	}{
		{"empty buffer", "", ModeNatural},
		{"slash prefix", "/help", ModeSlash},
		{"slash alone", "/", ModeSlash},
		{"backslash prefix", "\\select * from t", ModeSQL},
		{"backslash alone", "\\", ModeSQL},
		{"natural ASCII", "hello", ModeNatural},
		{"natural digit", "1234", ModeNatural},
		{"natural CJK first", "你好", ModeNatural},
		{"natural space first", " /help", ModeNatural},
		// Middle '/' does NOT switch mode — DeriveMode looks only at byte 0.
		{"middle slash natural", "hello /world", ModeNatural},
		{"middle backslash natural", "hello \\n", ModeNatural},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := DeriveMode(c.buffer); got != c.want {
				t.Errorf("DeriveMode(%q) = %v, want %v", c.buffer, got, c.want)
			}
		})
	}
}

// --- T1-16..T1-20 ValueWithoutPrefix ---

func TestValueWithoutPrefix(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		buffer string
		want   string
	}{
		{"natural unchanged", "hello world", "hello world"},
		{"natural empty", "", ""},
		{"slash strip", "/help", "help"},
		{"slash alone", "/", ""},
		{"sql strip", "\\select * from t", "select * from t"},
		{"sql alone", "\\", ""},
		{"slash multi-char", "/help arg1 arg2", "help arg1 arg2"},
		{"natural CJK", "你好世界", "你好世界"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := ValueWithoutPrefix(c.buffer); got != c.want {
				t.Errorf("ValueWithoutPrefix(%q) = %q, want %q", c.buffer, got, c.want)
			}
		})
	}
}
