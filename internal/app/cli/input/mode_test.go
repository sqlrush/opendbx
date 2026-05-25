// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package input

import "testing"

// --- T1-1..T1-5 InputMode enum ---

func TestInputMode_String(t *testing.T) {
	t.Parallel()
	cases := []struct {
		mode InputMode
		want string
	}{
		{InputModeNatural, "natural"},
		{InputModeSlash, "slash"},
		{InputModeSQL, "sql"},
		{InputMode(99), "natural"}, // unknown defaults to natural
	}
	for _, c := range cases {
		if got := c.mode.String(); got != c.want {
			t.Errorf("InputMode(%d).String() = %q, want %q", c.mode, got, c.want)
		}
	}
}

func TestInputMode_TriggerRune(t *testing.T) {
	t.Parallel()
	cases := []struct {
		mode InputMode
		want rune
	}{
		{InputModeNatural, 0},
		{InputModeSlash, '/'},
		{InputModeSQL, '\\'},
	}
	for _, c := range cases {
		if got := c.mode.TriggerRune(); got != c.want {
			t.Errorf("%v.TriggerRune() = %q, want %q", c.mode, got, c.want)
		}
	}
}

func TestInputMode_ZeroValue(t *testing.T) {
	t.Parallel()
	var m InputMode
	if m != InputModeNatural {
		t.Errorf("zero value InputMode = %v, want InputModeNatural", m)
	}
}

// --- T1-6..T1-15 DeriveMode (R2 C2 ★A single SoT) ---

func TestDeriveMode(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		buffer string
		want   InputMode
	}{
		{"empty buffer", "", InputModeNatural},
		{"slash prefix", "/help", InputModeSlash},
		{"slash alone", "/", InputModeSlash},
		{"backslash prefix", "\\select * from t", InputModeSQL},
		{"backslash alone", "\\", InputModeSQL},
		{"natural ASCII", "hello", InputModeNatural},
		{"natural digit", "1234", InputModeNatural},
		{"natural CJK first", "你好", InputModeNatural},
		{"natural space first", " /help", InputModeNatural},
		// Middle '/' does NOT switch mode — DeriveMode looks only at byte 0.
		{"middle slash natural", "hello /world", InputModeNatural},
		{"middle backslash natural", "hello \\n", InputModeNatural},
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
